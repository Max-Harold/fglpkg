// Package audit cross-checks installed Java JAR dependencies against a
// public vulnerability database (OSV.dev) and returns findings.
// v1 is report-only and queries OSV.dev only; BDL packages are not
// covered yet because no public CVE feed indexes them.
package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/4js-mikefolcher/fglpkg/internal/lockfile"
)

const (
	// defaultURL is the OSV.dev single-package query endpoint. It returns
	// full vulnerability details (id, summary, severity, references) so
	// no follow-up requests are needed per finding.
	defaultURL = "https://api.osv.dev/v1/query"

	defaultTimeout = 30 * time.Second

	// SourceLabel is the value emitted as `source` in the JSON report.
	SourceLabel = "osv.dev"
)

// Severity buckets, ordered from least to most severe. SeverityRank
// returns the ordinal; callers compare against a threshold to decide
// whether a finding fails the build.
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Finding is one vulnerability against one JAR coordinate.
type Finding struct {
	Coordinate  string  `json:"coordinate"` // pkg:maven/<groupId>/<artifactId>@<version>
	GroupID     string  `json:"groupId"`
	ArtifactID  string  `json:"artifactId"`
	Version     string  `json:"version"`
	ID          string  `json:"id"` // OSV/GHSA advisory id
	CVE         string  `json:"cve,omitempty"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	CVSSScore   float64 `json:"cvssScore,omitempty"`
	CVSSVector  string  `json:"cvssVector,omitempty"`
	Severity    string  `json:"severity"` // critical|high|medium|low
	Reference   string  `json:"reference,omitempty"`
}

// Options configure an Audit call. Zero values pick sensible defaults
// suitable for production use; tests inject URL and HTTPClient.
type Options struct {
	URL        string
	HTTPClient *http.Client
}

// Audit queries the advisory service for every JAR in jars and returns
// the resulting findings. Returns a non-nil error if the service is
// unreachable, returns a non-2xx status, or yields malformed JSON;
// callers must treat any error as "audit failed" rather than "no
// findings", since a partial-failure report is worse than no report.
//
// OSV.dev's /v1/query endpoint takes one package per request, so this
// makes one HTTP call per deduplicated JAR coordinate. For typical
// projects (≤ ~30 JARs) this completes well under the configured
// timeout. A future revision may use /v1/querybatch + parallel detail
// fetches for larger trees.
func Audit(jars []lockfile.LockedJAR, opts Options) ([]Finding, error) {
	if len(jars) == 0 {
		return nil, nil
	}

	url := opts.URL
	if url == "" {
		url = defaultURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	// Dedup coordinates so the same JAR is not queried twice.
	type query struct {
		purl string
		jar  lockfile.LockedJAR
	}
	seen := make(map[string]bool, len(jars))
	queries := make([]query, 0, len(jars))
	for _, j := range jars {
		purl := mavenPURL(j.GroupID, j.ArtifactID, j.Version)
		if seen[purl] {
			continue
		}
		seen[purl] = true
		queries = append(queries, query{purl: purl, jar: j})
	}

	var findings []Finding
	for _, q := range queries {
		resp, err := queryOSV(client, url, q.purl)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.Vulns {
			findings = append(findings, vulnToFinding(q, v))
		}
	}
	return findings, nil
}

// SeverityFromGHSA maps the OSV `database_specific.severity` string
// (used by GHSA-sourced entries) to one of the four canonical buckets.
// Returns the empty string when the input is unrecognized, so callers
// can fall back to other signals.
func SeverityFromGHSA(s string) string {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return SeverityCritical
	case "HIGH":
		return SeverityHigh
	case "MODERATE", "MEDIUM":
		return SeverityMedium
	case "LOW":
		return SeverityLow
	}
	return ""
}

// SeverityRank returns an ordinal for severity strings so callers can
// compare against a threshold: low=1, medium=2, high=3, critical=4.
// An unrecognized severity yields 0, which is below every valid threshold.
func SeverityRank(sev string) int {
	switch sev {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// ValidSeverity reports whether sev is one of the four recognized
// severity strings.
func ValidSeverity(sev string) bool {
	return SeverityRank(sev) > 0
}

func mavenPURL(group, artifact, version string) string {
	return "pkg:maven/" + group + "/" + artifact + "@" + version
}

// vulnToFinding converts an OSV vulnerability record into our Finding
// shape. Severity comes from `database_specific.severity` when present
// (GHSA always sets it); otherwise the finding is reported with an
// empty severity bucket, which SeverityRank treats as below every
// threshold — better to surface the finding than to silently demote it
// below the floor and fail closed.
func vulnToFinding(q struct {
	purl string
	jar  lockfile.LockedJAR
}, v osvVulnerability) Finding {
	f := Finding{
		Coordinate:  q.purl,
		GroupID:     q.jar.GroupID,
		ArtifactID:  q.jar.ArtifactID,
		Version:     q.jar.Version,
		ID:          v.ID,
		Title:       v.Summary,
		Description: v.Details,
		Severity:    SeverityFromGHSA(v.DatabaseSpecific.Severity),
	}
	if f.Severity == "" {
		// Default unknown-severity findings to medium so they surface at
		// the default --severity=medium floor. Erring conservative: it
		// is better to fail the build on an unclassified CVE than to
		// silently pass it.
		f.Severity = SeverityMedium
	}
	// Prefer the first CVE alias as a human-friendly identifier.
	for _, a := range v.Aliases {
		if strings.HasPrefix(a, "CVE-") {
			f.CVE = a
			break
		}
	}
	// Prefer ADVISORY references; fall back to the first reference.
	for _, r := range v.References {
		if strings.EqualFold(r.Type, "ADVISORY") && r.URL != "" {
			f.Reference = r.URL
			break
		}
	}
	if f.Reference == "" {
		for _, r := range v.References {
			if r.URL != "" {
				f.Reference = r.URL
				break
			}
		}
	}
	// Surface the first CVSS vector if any (numeric score parsing is
	// intentionally not done here — we trust the GHSA severity label).
	for _, s := range v.Severity {
		if s.Score != "" {
			f.CVSSVector = s.Score
			break
		}
	}
	return f
}

// osvResponse is the JSON shape returned by POST /v1/query.
type osvResponse struct {
	Vulns []osvVulnerability `json:"vulns"`
}

type osvVulnerability struct {
	ID               string        `json:"id"`
	Summary          string        `json:"summary"`
	Details          string        `json:"details"`
	Aliases          []string      `json:"aliases"`
	Severity         []osvSeverity `json:"severity"`
	References       []osvRef      `json:"references"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"` // CVSS vector string
}

type osvRef struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func queryOSV(client *http.Client, url, purl string) (*osvResponse, error) {
	body, err := json.Marshal(map[string]any{
		"package": map[string]string{"purl": purl},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode audit request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build audit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OSV.dev request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OSV.dev response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OSV.dev returned HTTP %d for %s: %s",
			resp.StatusCode, purl, strings.TrimSpace(string(data)))
	}
	var out osvResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("invalid OSV.dev response: %w", err)
	}
	return &out, nil
}

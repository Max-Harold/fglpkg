package manifest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
)

// HasWebcomponents reports presence/absence of declared webcomponents.
func TestHasWebcomponents(t *testing.T) {
	none := &manifest.Manifest{Name: "p", Version: "1.0.0"}
	if none.HasWebcomponents() {
		t.Error("manifest with no webcomponents reported HasWebcomponents = true")
	}
	some := &manifest.Manifest{
		Name: "p", Version: "1.0.0",
		Webcomponents: []string{"MyWidget"},
	}
	if !some.HasWebcomponents() {
		t.Error("manifest with webcomponents reported HasWebcomponents = false")
	}
}

// HasBDLContent is the signal that triggers per-Genero variant fan-out.
// Pure-WC manifests must report false; any BDL marker must flip it true.
func TestHasBDLContent(t *testing.T) {
	pureWC := &manifest.Manifest{
		Name: "p", Version: "1.0.0",
		Webcomponents: []string{"MyWidget"},
	}
	if pureWC.HasBDLContent() {
		t.Error("pure-WC manifest reported HasBDLContent = true")
	}

	cases := map[string]func(*manifest.Manifest){
		"main":             func(m *manifest.Manifest) { m.Main = "Entry.42m" },
		"programs":         func(m *manifest.Manifest) { m.Programs = []string{"Main"} },
		"bin":              func(m *manifest.Manifest) { m.Bin = map[string]string{"go": "run.sh"} },
		"root":             func(m *manifest.Manifest) { m.Root = "src" },
		"files":            func(m *manifest.Manifest) { m.Files = []string{"*.42m"} },
		"dependencies-java": func(m *manifest.Manifest) {
			m.Dependencies.Java = []manifest.JavaDependency{{GroupID: "g", ArtifactID: "a", Version: "1"}}
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			m := &manifest.Manifest{Name: "p", Version: "1.0.0"}
			mut(m)
			if !m.HasBDLContent() {
				t.Errorf("%s should have made HasBDLContent return true", name)
			}
		})
	}
}

// COMPONENTTYPE names must match the documented lexical rule.
func TestValidateWebcomponentNameFormat(t *testing.T) {
	cases := []string{"", "has space", "has/slash", "-leading", "name.dot", "name!bang"}
	for _, bad := range cases {
		t.Run(bad, func(t *testing.T) {
			m := &manifest.Manifest{
				Name: "p", Version: "1.0.0",
				Webcomponents: []string{bad},
			}
			if err := m.Validate(); err == nil {
				t.Fatalf("expected validation error for COMPONENTTYPE %q", bad)
			}
		})
	}
}

func TestValidateWebcomponentAcceptsDigitLeading(t *testing.T) {
	m := &manifest.Manifest{
		Name: "p", Version: "1.0.0",
		Webcomponents: []string{"3DChart"},
	}
	if err := m.Validate(); err != nil {
		t.Errorf("3DChart should be valid: %v", err)
	}
}

func TestValidateWebcomponentDuplicateName(t *testing.T) {
	m := &manifest.Manifest{
		Name: "p", Version: "1.0.0",
		Webcomponents: []string{"Chart", "Chart"},
	}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate COMPONENTTYPE") {
		t.Fatalf("expected duplicate-COMPONENTTYPE error, got %v", err)
	}
}

// Mixed packages — webcomponents alongside BDL fields — must validate
// cleanly. This is the new behavior we're enabling.
func TestValidateMixedPackage(t *testing.T) {
	m := &manifest.Manifest{
		Name: "chart-3d", Version: "1.0.0",
		Description: "BDL wrapper + 3D chart widget",
		License:     "MIT",
		Repository:  "https://github.com/example/chart-3d",
		Author:      "test@example.com",
		Programs:    []string{"ChartDemo"},
		Webcomponents: []string{"3DChart"},
		Dependencies: manifest.Dependencies{
			Java: []manifest.JavaDependency{{GroupID: "g", ArtifactID: "a", Version: "1"}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("mixed manifest should validate: %v", err)
	}
	if err := m.ValidateForPublish(); err != nil {
		t.Fatalf("mixed manifest should validate for publish: %v", err)
	}
}

// Pure-WC packages still validate when no BDL fields are present.
func TestValidatePureWebcomponentHappyPath(t *testing.T) {
	m := &manifest.Manifest{
		Name: "chart-3d", Version: "1.0.0",
		Description:   "3D chart widget",
		License:       "MIT",
		Webcomponents: []string{"3DChart", "Heatmap"},
		Dependencies: manifest.Dependencies{
			FGL: map[string]string{"wc-theme-base": "^1.0.0"},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

// The legacy `"type": "webcomponent"` field on disk is accepted-but-ignored.
// A round-trip preserves it for backward compatibility with older scaffolds.
func TestTypeFieldAcceptedButIgnored(t *testing.T) {
	dir := t.TempDir()
	on := filepath.Join(dir, manifest.Filename)
	const raw = `{
  "name": "legacy-wc",
  "version": "1.0.0",
  "type": "webcomponent",
  "description": "Legacy manifest",
  "dependencies": { "fgl": {} },
  "webcomponents": ["MyWidget"]
}`
	if err := os.WriteFile(on, []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Type != "webcomponent" {
		t.Errorf("Type field not preserved on load: %q", m.Type)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("legacy manifest should validate: %v", err)
	}
	// HasBDLContent should be false — no BDL fields declared.
	if m.HasBDLContent() {
		t.Error("pure-WC manifest (with type=webcomponent on disk) reported HasBDLContent = true")
	}
}

func TestWebcomponentManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := &manifest.Manifest{
		Name: "chart-3d", Version: "1.0.0",
		Description:   "3D chart widget",
		Webcomponents: []string{"3DChart"},
		Dependencies:  manifest.Dependencies{FGL: map[string]string{}},
	}
	if err := orig.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Webcomponents) != 1 || got.Webcomponents[0] != "3DChart" {
		t.Fatalf("Webcomponents round-trip: got %v", got.Webcomponents)
	}
	// Saved manifest must NOT inject a "type" field (we only preserve it
	// when explicitly set on the in-memory struct).
	data, err := os.ReadFile(filepath.Join(dir, manifest.Filename))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), `"type"`) {
		t.Fatalf("did not expect `type` key in saved JSON:\n%s", data)
	}
}

func TestBDLManifestOmitsWebcomponentsField(t *testing.T) {
	m := &manifest.Manifest{
		Name: "p", Version: "1.0.0",
		Dependencies: manifest.Dependencies{FGL: map[string]string{}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"webcomponents"`) {
		t.Fatalf("expected no `webcomponents` key in BDL manifest JSON: %s", data)
	}
}

package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/4js-mikefolcher/fglpkg/internal/config"
	"github.com/4js-mikefolcher/fglpkg/internal/registry"
	"github.com/4js-mikefolcher/fglpkg/internal/resolver"
)

// RepositorySet fronts one or more providers and implements the routing +
// collision-guard policy (spec §6). It exposes Versions/Info methods shaped for
// resolver.NewWithFetchers.
//
// Routing per package name:
//   - pinned (a manifest registry: pin or a lockfile Source) → that provider only;
//   - otherwise query every admitting provider and count non-not-found hits:
//     0 → not found, 1 → resolve + record source, ≥2 → a hard collision error.
//
// The per-name decision is memoized so Versions and the later Info call route to
// the same provider without re-querying.
type RepositorySet struct {
	providers   []Provider                 // priority order (lowest priority value first)
	descriptors map[string]config.Registry // provider name → descriptor (for Admits)
	pins        map[string]string          // package name → required registry name
	restrictTo  string                     // if set, resolution is limited to this provider

	mu     sync.Mutex
	routes map[string]routeDecision
}

type routeDecision struct {
	provider Provider
	versions []resolver.CandidateVersion
}

// NewRepositorySet builds a set from providers (any order — sorted by the
// matching descriptor's Priority), the descriptors, and the per-name pins.
func NewRepositorySet(providers []Provider, descriptors []config.Registry, pins map[string]string) *RepositorySet {
	dmap := make(map[string]config.Registry, len(descriptors))
	for _, d := range descriptors {
		dmap[d.Name] = d
	}
	ordered := append([]Provider(nil), providers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return dmap[ordered[i].Name()].Priority < dmap[ordered[j].Name()].Priority
	})
	if pins == nil {
		pins = map[string]string{}
	}
	return &RepositorySet{
		providers:   ordered,
		descriptors: dmap,
		pins:        pins,
		routes:      map[string]routeDecision{},
	}
}

// Restrict limits resolution to the single named provider (the --registry flag).
func (rs *RepositorySet) Restrict(name string) { rs.restrictTo = name }

// Providers returns the providers in priority order.
func (rs *RepositorySet) Providers() []Provider { return rs.providers }

// Versions implements resolver.VersionFetcher.
func (rs *RepositorySet) Versions(name string) ([]resolver.CandidateVersion, error) {
	d, err := rs.route(name)
	if err != nil {
		return nil, err
	}
	return d.versions, nil
}

// Info implements resolver.InfoFetcher, routing to the same provider Versions did.
func (rs *RepositorySet) Info(name, version, generoMajor string) (*registry.PackageInfo, error) {
	d, err := rs.route(name)
	if err != nil {
		return nil, err
	}
	return d.provider.FetchInfo(name, version, generoMajor)
}

func (rs *RepositorySet) byName(name string) Provider {
	for _, p := range rs.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

func (rs *RepositorySet) admits(p Provider, name string) bool {
	d, ok := rs.descriptors[p.Name()]
	if !ok {
		return true
	}
	return d.Admits(name)
}

func (rs *RepositorySet) route(name string) (routeDecision, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if d, ok := rs.routes[name]; ok {
		return d, nil
	}

	// A --registry restriction forces a single provider.
	if rs.restrictTo != "" {
		p := rs.byName(rs.restrictTo)
		if p == nil {
			return routeDecision{}, fmt.Errorf("registry %q is not configured", rs.restrictTo)
		}
		vs, err := p.FetchVersions(name)
		if err != nil {
			return routeDecision{}, err
		}
		d := routeDecision{provider: p, versions: vs}
		rs.routes[name] = d
		return d, nil
	}

	// Pinned name → that provider only (deterministic short-circuit).
	if pin := rs.pins[name]; pin != "" {
		p := rs.byName(pin)
		if p == nil {
			return routeDecision{}, fmt.Errorf(
				"package %q is pinned to registry %q, which is not configured", name, pin)
		}
		vs, err := p.FetchVersions(name)
		if err != nil {
			if errors.Is(err, registry.ErrNotFound) {
				return routeDecision{}, fmt.Errorf(
					"package %q is pinned to registry %q but was not found there", name, pin)
			}
			return routeDecision{}, err
		}
		d := routeDecision{provider: p, versions: vs}
		rs.routes[name] = d
		return d, nil
	}

	// Unpinned → query all admitting providers; count hits.
	var hits []routeDecision
	var searched []string
	for _, p := range rs.providers {
		if !rs.admits(p, name) {
			continue
		}
		searched = append(searched, p.Name())
		vs, err := p.FetchVersions(name)
		if err != nil {
			if errors.Is(err, registry.ErrNotFound) {
				continue
			}
			// Auth or other hard error: abort. Never silently drop a repo from
			// the hit count — that could let a package mis-route (spec §7.2).
			return routeDecision{}, err
		}
		hits = append(hits, routeDecision{provider: p, versions: vs})
	}

	switch len(hits) {
	case 0:
		return routeDecision{}, fmt.Errorf(
			"package %q not found in any configured repository (%s): %w",
			name, strings.Join(searched, ", "), registry.ErrNotFound)
	case 1:
		rs.routes[name] = hits[0]
		return hits[0], nil
	default:
		return routeDecision{}, collisionError(name, hits)
	}
}

// collisionError builds the disambiguation message for a name present in more
// than one repository (spec §6).
func collisionError(name string, hits []routeDecision) error {
	var b strings.Builder
	fmt.Fprintf(&b, "package %q is available from more than one repository:\n", name)
	first := ""
	for _, h := range hits {
		vers := make([]string, 0, len(h.versions))
		for _, v := range h.versions {
			vers = append(vers, v.Version.String())
		}
		if first == "" {
			first = h.provider.Name()
		}
		fmt.Fprintf(&b, "    %-14s %s\n", h.provider.Name(), strings.Join(vers, ", "))
	}
	fmt.Fprintf(&b, "  Refusing to guess. Pin the source in fglpkg.json:\n")
	fmt.Fprintf(&b, "      \"dependencies\": { \"fgl\": { %q: { \"version\": \"^1.0.0\", \"registry\": %q } } }\n", name, first)
	fmt.Fprintf(&b, "  or rename so the name is unique to one repository.")
	return errors.New(b.String())
}

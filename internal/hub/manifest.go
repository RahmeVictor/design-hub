// Package hub implements the design-hub tool: a local server for a repo's
// design/ directory with an embedded design↔implementation compare page, a
// manifest preflight (doctor), and a headless-Chrome screenshot/pixel-diff
// runner (shots). The manifest is <design dir>/compare/screens.json.
package hub

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestPath is the manifest location relative to the design dir.
const ManifestPath = "compare/screens.json"

type Device struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Surface struct {
	Label string `json:"label"`
	Run   string `json:"run"`
}

type Screen struct {
	Surface   string `json:"surface"`
	Screen    string `json:"screen"`
	Device    string `json:"device"`
	DesignURL string `json:"designUrl"`
	ImplURL   string `json:"implUrl"`
	DesignNav string `json:"designNav"`
	ImplNav   string `json:"implNav"`
}

// Manifest is the screens.json schema. Only Devices and Screens are required;
// everything else has a sensible default so older manifests keep working.
type Manifest struct {
	Title    string             `json:"title"`
	Tokens   []string           `json:"tokens"`
	Favicon  string             `json:"favicon"`
	Parity   string             `json:"parity"`
	Devices  map[string]Device  `json:"devices"`
	Surfaces map[string]Surface `json:"surfaces"`
	Screens  []Screen           `json:"screens"`
}

// LoadManifest reads and parses <dir>/compare/screens.json.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, filepath.FromSlash(ManifestPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Severity of a validation problem.
type Severity string

const (
	Warn Severity = "warn"
	Fail Severity = "fail"
)

type Problem struct {
	Severity Severity
	Message  string
}

// Validate checks the manifest's internal consistency and that every file it
// references (designUrl, tokens, favicon, parity) exists under dir.
func (m *Manifest) Validate(dir string) []Problem {
	var probs []Problem
	fail := func(format string, a ...any) {
		probs = append(probs, Problem{Fail, fmt.Sprintf(format, a...)})
	}
	warn := func(format string, a ...any) {
		probs = append(probs, Problem{Warn, fmt.Sprintf(format, a...)})
	}

	if len(m.Devices) == 0 {
		fail("no devices defined")
	}
	for name, d := range m.Devices {
		if d.Width <= 0 || d.Height <= 0 {
			fail("device %q has non-positive dimensions (%dx%d)", name, d.Width, d.Height)
		}
	}
	if len(m.Screens) == 0 {
		fail("no screens defined")
	}

	seen := map[string]bool{}
	for i, s := range m.Screens {
		id := s.Surface + "/" + s.Screen
		switch {
		case s.Surface == "" || s.Screen == "":
			fail("screens[%d]: surface and screen are required", i)
		case seen[id]:
			fail("duplicate screen %q", id)
		default:
			seen[id] = true
		}
		if _, ok := m.Devices[s.Device]; !ok {
			fail("screen %q: unknown device %q", id, s.Device)
		}
		if len(m.Surfaces) > 0 {
			if _, ok := m.Surfaces[s.Surface]; !ok {
				warn("screen %q: surface %q has no entry in surfaces", id, s.Surface)
			}
		}
		if s.DesignURL != "" {
			if !strings.HasPrefix(s.DesignURL, "/") {
				fail("screen %q: designUrl must be hub-root-relative (start with /): %q", id, s.DesignURL)
			} else if p := hubPathOnDisk(dir, s.DesignURL); p != "" {
				if _, err := os.Stat(p); err != nil {
					fail("screen %q: designUrl file missing: %s", id, p)
				}
			}
		}
	}

	for _, t := range m.Tokens {
		if !strings.HasPrefix(t, "/") {
			fail("tokens entry must be hub-root-relative (start with /): %q", t)
		} else if p := hubPathOnDisk(dir, t); p != "" {
			if _, err := os.Stat(p); err != nil {
				fail("tokens file missing: %s", p)
			}
		}
	}
	if m.Favicon != "" {
		if p := hubPathOnDisk(dir, m.Favicon); p == "" {
			fail("favicon must be hub-root-relative (start with /): %q", m.Favicon)
		} else if _, err := os.Stat(p); err != nil {
			fail("favicon file missing: %s", p)
		}
	}
	if m.Parity != "" {
		p := filepath.Join(dir, "compare", filepath.FromSlash(m.Parity))
		if _, err := os.Stat(p); err != nil {
			fail("parity file missing: %s", p)
		}
	}
	return probs
}

// hubPathOnDisk maps a hub-root-relative URL ("/prototype/X.html", possibly
// percent-encoded, possibly carrying a query/fragment) to a path under dir.
// Returns "" if the URL is not hub-root-relative or escapes dir.
func hubPathOnDisk(dir, raw string) string {
	if !strings.HasPrefix(raw, "/") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	rel := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
	p := filepath.Join(dir, rel)
	if abs, err := filepath.Abs(p); err == nil {
		if absDir, err := filepath.Abs(dir); err == nil {
			if !strings.HasPrefix(abs, absDir+string(os.PathSeparator)) && abs != absDir {
				return ""
			}
		}
	}
	return p
}

// ImplOrigins returns the distinct implUrl origins mapped to the sorted list
// of surfaces that use each.
func (m *Manifest) ImplOrigins() map[string][]string {
	surfaces := map[string]map[string]bool{}
	for _, s := range m.Screens {
		if s.ImplURL == "" {
			continue
		}
		u, err := url.Parse(s.ImplURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		origin := u.Scheme + "://" + u.Host
		if surfaces[origin] == nil {
			surfaces[origin] = map[string]bool{}
		}
		surfaces[origin][s.Surface] = true
	}
	out := map[string][]string{}
	for origin, set := range surfaces {
		var list []string
		for s := range set {
			list = append(list, s)
		}
		sort.Strings(list)
		out[origin] = list
	}
	return out
}

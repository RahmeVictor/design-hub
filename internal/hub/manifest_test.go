package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestGood(t *testing.T) {
	m, err := LoadManifest(filepath.Join("..", "..", "testdata", "design"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Screens) != 2 || len(m.Devices) != 2 || m.Title == "" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if probs := m.Validate(filepath.Join("..", "..", "testdata", "design")); len(probs) != 0 {
		t.Fatalf("expected clean validation, got %v", probs)
	}
}

// writeDesignDir creates a minimal design dir from a manifest, creating any
// designUrl/tokens files the manifest references unless listed in missing.
func writeDesignDir(t *testing.T, m *Manifest, missing ...string) string {
	t.Helper()
	dir := t.TempDir()
	skip := map[string]bool{}
	for _, p := range missing {
		skip[p] = true
	}
	touch := func(rel string) {
		if rel == "" || skip[rel] {
			return
		}
		p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	touch("/" + ManifestPath)
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(ManifestPath)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, s := range m.Screens {
		touch(s.DesignURL)
	}
	for _, tok := range m.Tokens {
		touch(tok)
	}
	if m.Parity != "" {
		touch("/compare/" + m.Parity)
	}
	return dir
}

func baseManifest() *Manifest {
	return &Manifest{
		Devices: map[string]Device{"desktop": {Width: 800, Height: 600}},
		Screens: []Screen{
			{Surface: "web", Screen: "home", Device: "desktop", DesignURL: "/prototype/home.html"},
		},
	}
}

func problemsContain(probs []Problem, sev Severity, substr string) bool {
	for _, p := range probs {
		if p.Severity == sev && strings.Contains(p.Message, substr) {
			return true
		}
	}
	return false
}

func TestValidateUnknownDevice(t *testing.T) {
	m := baseManifest()
	m.Screens[0].Device = "tablet"
	dir := writeDesignDir(t, m)
	if probs := m.Validate(dir); !problemsContain(probs, Fail, "unknown device") {
		t.Fatalf("expected unknown-device fail, got %v", probs)
	}
}

func TestValidateDuplicateScreen(t *testing.T) {
	m := baseManifest()
	m.Screens = append(m.Screens, m.Screens[0])
	dir := writeDesignDir(t, m)
	if probs := m.Validate(dir); !problemsContain(probs, Fail, "duplicate screen") {
		t.Fatalf("expected duplicate-screen fail, got %v", probs)
	}
}

func TestValidateMissingDesignFile(t *testing.T) {
	m := baseManifest()
	dir := writeDesignDir(t, m, "/prototype/home.html")
	if probs := m.Validate(dir); !problemsContain(probs, Fail, "designUrl file missing") {
		t.Fatalf("expected missing designUrl fail, got %v", probs)
	}
}

func TestValidateRelativeDesignURL(t *testing.T) {
	m := baseManifest()
	m.Screens[0].DesignURL = "prototype/home.html"
	dir := writeDesignDir(t, m)
	if probs := m.Validate(dir); !problemsContain(probs, Fail, "hub-root-relative") {
		t.Fatalf("expected hub-root-relative fail, got %v", probs)
	}
}

func TestValidateMissingToken(t *testing.T) {
	m := baseManifest()
	m.Tokens = []string{"/prototype/tokens/colors.css"}
	dir := writeDesignDir(t, m, "/prototype/tokens/colors.css")
	if probs := m.Validate(dir); !problemsContain(probs, Fail, "tokens file missing") {
		t.Fatalf("expected missing token fail, got %v", probs)
	}
}

func TestValidateSurfaceWarn(t *testing.T) {
	m := baseManifest()
	m.Surfaces = map[string]Surface{"other": {Label: "Other"}}
	dir := writeDesignDir(t, m)
	if probs := m.Validate(dir); !problemsContain(probs, Warn, "no entry in surfaces") {
		t.Fatalf("expected surface warn, got %v", probs)
	}
}

func TestValidatePercentEncodedDesignURL(t *testing.T) {
	m := baseManifest()
	m.Screens[0].DesignURL = "/prototype/My%20Screen.html"
	dir := writeDesignDir(t, m, "/prototype/My%20Screen.html")
	p := filepath.Join(dir, "prototype", "My Screen.html")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if probs := m.Validate(dir); len(probs) != 0 {
		t.Fatalf("percent-encoded URL should resolve to the decoded file, got %v", probs)
	}
}

func TestImplOrigins(t *testing.T) {
	m := baseManifest()
	m.Screens = []Screen{
		{Surface: "a", Screen: "s1", Device: "desktop", DesignURL: "/p/a.html", ImplURL: "http://localhost:8000/app/x"},
		{Surface: "b", Screen: "s2", Device: "desktop", DesignURL: "/p/b.html", ImplURL: "http://localhost:8000/app/y"},
		{Surface: "c", Screen: "s3", Device: "desktop", DesignURL: "/p/c.html", ImplURL: "http://localhost:3000/"},
		{Surface: "d", Screen: "s4", Device: "desktop", DesignURL: "/p/d.html", ImplURL: ""},
	}
	got := m.ImplOrigins()
	if len(got) != 2 {
		t.Fatalf("expected 2 origins, got %v", got)
	}
	if s := got["http://localhost:8000"]; len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Fatalf("unexpected surfaces for :8000: %v", s)
	}
}

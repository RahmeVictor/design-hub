package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctorCleanFixture(t *testing.T) {
	var out strings.Builder
	code := Doctor(&out, DoctorOptions{Dir: "../../testdata/design"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "SUMMARY ok=1 warn=0 fail=0") {
		t.Fatalf("unexpected summary:\n%s", out.String())
	}
}

func TestDoctorImplDownIsWarn(t *testing.T) {
	m := baseManifest()
	m.Surfaces = map[string]Surface{"web": {Label: "Web", Run: "make dev"}}
	// A port from the dynamic range that nothing in the test binds.
	m.Screens[0].ImplURL = "http://127.0.0.1:59999/"
	dir := writeDesignDir(t, m)

	var out strings.Builder
	code := Doctor(&out, DoctorOptions{Dir: dir})
	if code != 0 {
		t.Fatalf("impl-down should stay exit 0 without -require-impl, got %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "DOWN") || !strings.Contains(out.String(), "run: make dev") {
		t.Fatalf("expected DOWN warn with run hint:\n%s", out.String())
	}

	out.Reset()
	code = Doctor(&out, DoctorOptions{Dir: dir, RequireImpl: true})
	if code != 1 {
		t.Fatalf("-require-impl should make impl-down exit 1, got %d:\n%s", code, out.String())
	}
}

func TestDoctorImplUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	m := baseManifest()
	m.Screens[0].ImplURL = srv.URL + "/app/"
	dir := writeDesignDir(t, m)

	var out strings.Builder
	if code := Doctor(&out, DoctorOptions{Dir: dir}); code != 0 {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "up — surfaces: web") {
		t.Fatalf("expected impl-up line:\n%s", out.String())
	}
}

func TestDoctorBrokenManifest(t *testing.T) {
	var out strings.Builder
	if code := Doctor(&out, DoctorOptions{Dir: t.TempDir()}); code != 2 {
		t.Fatalf("missing manifest should exit 2, got %d:\n%s", code, out.String())
	}
}

func TestDoctorValidationFail(t *testing.T) {
	m := baseManifest()
	dir := writeDesignDir(t, m, "/prototype/home.html")
	var out strings.Builder
	if code := Doctor(&out, DoctorOptions{Dir: dir}); code != 1 {
		t.Fatalf("missing design file should exit 1, got %d:\n%s", code, out.String())
	}
}

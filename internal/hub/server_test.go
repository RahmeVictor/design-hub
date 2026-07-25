package hub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestServerRootRedirect(t *testing.T) {
	h := Handler("../../testdata/design", "")
	resp := get(t, h, "/")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/compare/" {
		t.Fatalf("expected 302 → /compare/, got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestServerEmbeddedComparePage(t *testing.T) {
	h := Handler("../../testdata/design", "")
	for _, path := range []string{"/compare/", "/compare/index.html"} {
		resp := get(t, h, path)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		if !strings.Contains(string(body), "designhub.mode") {
			t.Fatalf("%s: expected the embedded compare page", path)
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: expected no-store", path)
		}
	}
}

func TestServerDiskFallthrough(t *testing.T) {
	h := Handler("../../testdata/design", "")
	resp := get(t, h, "/compare/screens.json")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "\"screens\"") {
		t.Fatalf("screens.json should come from disk, got %d: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("expected no-store on disk files")
	}
	resp = get(t, h, "/prototype/tokens/colors.css")
	if resp.StatusCode != 200 {
		t.Fatalf("prototype files should be served, got %d", resp.StatusCode)
	}
}

func TestServerUIDirOverride(t *testing.T) {
	uiDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(uiDir, "compare"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "<html>custom compare page</html>"
	if err := os.WriteFile(filepath.Join(uiDir, "compare", "index.html"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	h := Handler("../../testdata/design", uiDir)
	resp := get(t, h, "/compare/")
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "custom compare page") {
		t.Fatalf("-ui dir should override the embedded page, got: %s", body)
	}
}

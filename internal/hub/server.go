package hub

import (
	_ "embed"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// compareHTML is the embedded design↔implementation compare page. It is the
// canonical UI: a repo's own compare/index.html (if any) is shadowed, so the
// page can evolve with the tool instead of drifting per project.
//
//go:embed ui/compare/index.html
var compareHTML []byte

// Handler serves the design dir with the embedded compare page at /compare/.
// uiDir, when non-empty, serves the compare page from <uiDir>/compare/index.html
// instead of the embedded copy (escape hatch for hacking on the UI).
func Handler(dir, uiDir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/compare/", http.StatusFound)
			return
		}
		// The hub iterates on live-edited files; never let the browser cache.
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/compare/" || r.URL.Path == "/compare/index.html" {
			if uiDir != "" {
				http.ServeFile(w, r, filepath.Join(uiDir, "compare", "index.html"))
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(compareHTML)
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux
}

// Serve runs the hub server. Dev-only tooling: never deployed, no auth, binds
// localhost by default.
func Serve(addr, dir, uiDir string) error {
	if _, err := os.Stat(filepath.Join(dir, "compare", "index.html")); err == nil && uiDir == "" {
		log.Printf("note: %s/compare/index.html exists but is shadowed by the embedded compare page (use -ui to serve a page from disk)", dir)
	}
	if _, err := LoadManifest(dir); err != nil {
		log.Printf("warning: %v — the compare page will be empty (run `designhub doctor`)", err)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(dir, uiDir),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("design hub: http://%s/compare/ (serving %s)", addr, dir)
	return srv.ListenAndServe()
}

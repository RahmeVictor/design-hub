package hub

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ShotsOptions configures a screenshot run.
type ShotsOptions struct {
	Dir     string
	Out     string // default <dir>/compare/shots
	Hub     string // running hub base URL, default http://localhost:4400
	Chrome  string // chrome binary; falls back to $CHROME, then auto-detect
	Only    string // "surface" or "surface/screen-substring" filter
	Diff    bool   // also produce pixel diffs + scores
	MaxDiff float64
	// Tolerance is the per-channel pixel tolerance for --diff (default 16).
	Tolerance uint8
}

// ShotReport is one row of report.json.
type ShotReport struct {
	Surface           string   `json:"surface"`
	Screen            string   `json:"screen"`
	Slug              string   `json:"slug"`
	Design            string   `json:"design,omitempty"`
	Impl              string   `json:"impl,omitempty"`
	Diff              string   `json:"diff,omitempty"`
	ScorePercent      *float64 `json:"scorePercent,omitempty"`
	DimensionMismatch bool     `json:"dimensionMismatch,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"google-chrome",
	"chromium",
}

// FindChrome resolves the Chrome binary: explicit path, $CHROME, then the
// auto-detect list. Returns "" when none is found.
func FindChrome(explicit string) string {
	for _, c := range []string{explicit, os.Getenv("CHROME")} {
		if c != "" {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}
	for _, c := range chromeCandidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(surface, screen string) string {
	return strings.Trim(slugRe.ReplaceAllString(surface+"-"+screen, "-"), "-")
}

// Shots captures design + implementation screenshots for every manifest entry
// (optionally filtered), writes a gallery and report.json, and — with Diff —
// pixel-diff images and scores. Returns the exit code: 0 clean, 1 any shot
// failed or a score exceeded MaxDiff, 2 usage/environment error.
func Shots(w io.Writer, opts ShotsOptions) int {
	if opts.Hub == "" {
		opts.Hub = "http://localhost:4400"
	}
	if opts.Out == "" {
		opts.Out = filepath.Join(opts.Dir, "compare", "shots")
	}
	if opts.Tolerance == 0 {
		opts.Tolerance = 16
	}

	m, err := LoadManifest(opts.Dir)
	if err != nil {
		fmt.Fprintf(w, "error: %v\n", err)
		return 2
	}
	chrome := FindChrome(opts.Chrome)
	if chrome == "" {
		fmt.Fprintln(w, "error: no Chrome found — pass -chrome or set CHROME=/path/to/chrome")
		return 2
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if resp, err := client.Get(opts.Hub + "/compare/screens.json"); err != nil {
		fmt.Fprintf(w, "error: design hub not reachable at %s — run the serve command first\n", opts.Hub)
		return 2
	} else {
		resp.Body.Close()
	}
	if err := os.MkdirAll(opts.Out, 0o755); err != nil {
		fmt.Fprintf(w, "error: %v\n", err)
		return 2
	}

	onlySurface, onlyScreen := opts.Only, ""
	if i := strings.IndexByte(opts.Only, '/'); i >= 0 {
		onlySurface, onlyScreen = opts.Only[:i], opts.Only[i+1:]
	}

	var reports []ShotReport
	failed := false
	for _, s := range m.Screens {
		if onlySurface != "" && s.Surface != onlySurface {
			continue
		}
		if onlyScreen != "" && !strings.Contains(s.Screen, onlyScreen) {
			continue
		}
		device, ok := m.Devices[s.Device]
		if !ok {
			continue // doctor reports this; don't crash a shots run on it
		}
		r := ShotReport{Surface: s.Surface, Screen: s.Screen, Slug: slugify(s.Surface, s.Screen)}
		fmt.Fprintf(w, "%s / %s\n", s.Surface, s.Screen)

		shoot := func(url, suffix string) string {
			out := filepath.Join(opts.Out, r.Slug+"-"+suffix+".png")
			if err := screenshot(chrome, url, out, device); err != nil {
				fmt.Fprintf(w, "  x %s (%s: %v)\n", filepath.Base(out), url, err)
				r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", suffix, err))
				failed = true
				return ""
			}
			fmt.Fprintf(w, "  + %s\n", filepath.Base(out))
			return filepath.Base(out)
		}
		if s.DesignURL != "" {
			r.Design = shoot(opts.Hub+s.DesignURL, "design")
		}
		if s.ImplURL != "" {
			r.Impl = shoot(s.ImplURL, "impl")
		}

		if opts.Diff && r.Design != "" && r.Impl != "" {
			diffFile := filepath.Join(opts.Out, r.Slug+"-diff.png")
			res, err := DiffPNG(filepath.Join(opts.Out, r.Design), filepath.Join(opts.Out, r.Impl), diffFile, opts.Tolerance)
			if err != nil {
				r.Errors = append(r.Errors, fmt.Sprintf("diff: %v", err))
				failed = true
			} else {
				pct := res.Score * 100
				r.ScorePercent = &pct
				r.Diff = filepath.Base(diffFile)
				r.DimensionMismatch = res.DimensionMismatch
				fmt.Fprintf(w, "  = %.2f%% mismatch%s\n", pct, mismatchNote(res))
				if opts.MaxDiff > 0 && pct > opts.MaxDiff {
					failed = true
				}
			}
		}
		reports = append(reports, r)
	}

	if err := writeReport(opts.Out, reports); err != nil {
		fmt.Fprintf(w, "error: %v\n", err)
		return 2
	}
	if err := writeGallery(opts.Out, m.Title, reports); err != nil {
		fmt.Fprintf(w, "error: %v\n", err)
		return 2
	}
	if opts.Out == filepath.Join(opts.Dir, "compare", "shots") {
		fmt.Fprintf(w, "gallery: %s/compare/shots/index.html\n", opts.Hub)
	} else {
		fmt.Fprintf(w, "gallery: %s\n", filepath.Join(opts.Out, "index.html"))
	}
	if failed {
		return 1
	}
	return 0
}

func mismatchNote(res DiffResult) string {
	if res.DimensionMismatch {
		return " (dimension mismatch — compared the intersection)"
	}
	return ""
}

func screenshot(chrome, url, out string, d Device) error {
	cmd := exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", d.Width, d.Height),
		"--screenshot="+out, url)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chrome exited: %w", err)
	}
	if _, err := os.Stat(out); err != nil {
		return fmt.Errorf("no screenshot produced (url unreachable?)")
	}
	return nil
}

func writeReport(out string, reports []ShotReport) error {
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "report.json"), append(data, '\n'), 0o644)
}

func writeGallery(out, title string, reports []ShotReport) error {
	if title == "" {
		title = "Design ↔ implementation shots"
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><meta charset=\"utf-8\"><title>" + html.EscapeString(title) + "</title>\n")
	b.WriteString(`<style>body{font-family:system-ui;background:#f4f2ef;margin:24px}h2{font-size:15px;margin:26px 0 8px}
.trio{display:flex;gap:12px}.trio figure{margin:0;flex:1;min-width:0}.trio img{width:100%;border:1px solid #ddd;border-radius:10px;background:#fff}
figcaption{font-size:11px;color:#888;text-transform:uppercase;letter-spacing:.06em;font-weight:700;margin-bottom:5px}
.score{display:inline-block;font-size:11px;font-weight:700;border-radius:999px;padding:2px 10px;margin-left:8px;background:#e7f5e7;color:#1c7c1c}
.score.bad{background:#fde8e8;color:#c62828}.err{font-size:12px;color:#c62828}</style>
`)
	fmt.Fprintf(&b, "<h1 style='font-size:19px'>%s</h1>\n", html.EscapeString(title))
	for _, r := range reports {
		fmt.Fprintf(&b, "<h2>%s / %s", html.EscapeString(r.Surface), html.EscapeString(r.Screen))
		if r.ScorePercent != nil {
			cls := ""
			if *r.ScorePercent >= 5 {
				cls = " bad"
			}
			fmt.Fprintf(&b, "<span class='score%s'>%.2f%% mismatch</span>", cls, *r.ScorePercent)
		}
		b.WriteString("</h2>\n<div class='trio'>\n")
		cell := func(caption, file string) {
			if file == "" {
				return
			}
			fmt.Fprintf(&b, "<figure><figcaption>%s</figcaption><img src='%s'></figure>\n",
				caption, html.EscapeString(file))
		}
		cell("design", r.Design)
		cell("implementation", r.Impl)
		cell("diff", r.Diff)
		b.WriteString("</div>\n")
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "<p class='err'>%s</p>\n", html.EscapeString(e))
		}
	}
	return os.WriteFile(filepath.Join(out, "index.html"), []byte(b.String()), 0o644)
}

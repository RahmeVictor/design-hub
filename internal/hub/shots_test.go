package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[[2]string]string{
		{"rm", "dashboard"}:       "rm-dashboard",
		{"client", "order flow!"}: "client-order-flow",
		{"admin", "users / fin"}:  "admin-users-fin",
		{"brand", "logo (dark) "}: "brand-logo-dark",
	}
	for in, want := range cases {
		if got := slugify(in[0], in[1]); got != want {
			t.Errorf("slugify(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestGalleryAndReport(t *testing.T) {
	out := t.TempDir()
	score := 7.25
	reports := []ShotReport{
		{Surface: "web", Screen: "home", Slug: "web-home", Design: "web-home-design.png",
			Impl: "web-home-impl.png", Diff: "web-home-diff.png", ScorePercent: &score},
		{Surface: "web", Screen: "docs", Slug: "web-docs", Errors: []string{"design: chrome exited"}},
	}
	if err := writeReport(out, reports); err != nil {
		t.Fatal(err)
	}
	if err := writeGallery(out, "Fixture", reports); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "\"scorePercent\": 7.25") {
		t.Fatalf("report.json missing score: %s", report)
	}
	gallery, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"7.25% mismatch", "score bad", "chrome exited", "web-home-diff.png"} {
		if !strings.Contains(string(gallery), want) {
			t.Fatalf("gallery missing %q:\n%s", want, gallery)
		}
	}
}

func TestFindChromeMissingIsEmpty(t *testing.T) {
	t.Setenv("CHROME", "")
	t.Setenv("PATH", t.TempDir()) // hide any real chrome on PATH
	if runtimeHasMacChrome() {
		t.Skip("macOS Chrome app present — auto-detect will find it")
	}
	if got := FindChrome(""); got != "" {
		t.Fatalf("expected no chrome, got %q", got)
	}
}

func runtimeHasMacChrome() bool {
	_, err := os.Stat(chromeCandidates[0])
	return err == nil
}

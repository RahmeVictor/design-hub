package hub

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DoctorOptions configures the preflight run.
type DoctorOptions struct {
	Dir         string
	ImplTimeout time.Duration
	RequireImpl bool // promote unreachable impl origins from warn to fail
}

// Doctor validates the manifest and probes the implementation origins, writing
// a human-readable checklist to w. Returns the exit code: 0 clean (warnings
// allowed), 1 problems, 2 usage/layout error.
func Doctor(w io.Writer, opts DoctorOptions) int {
	if opts.ImplTimeout <= 0 {
		opts.ImplTimeout = 3 * time.Second
	}
	ok, warns, fails := 0, 0, 0
	report := func(sev Severity, format string, a ...any) {
		switch sev {
		case Warn:
			warns++
		case Fail:
			fails++
		}
		fmt.Fprintf(w, "%-5s %s\n", string(sev), fmt.Sprintf(format, a...))
	}

	m, err := LoadManifest(opts.Dir)
	if err != nil {
		fmt.Fprintf(w, "fail  %v\n", err)
		fmt.Fprintf(w, "\nSUMMARY ok=0 warn=0 fail=1\n")
		return 2
	}

	probs := m.Validate(opts.Dir)
	for _, p := range probs {
		report(p.Severity, "%s", p.Message)
	}
	if len(probs) == 0 {
		ok++
		fmt.Fprintf(w, "ok    manifest: %d screens, %d surfaces, %d devices — all referenced files exist\n",
			len(m.Screens), len(m.Surfaces), len(m.Devices))
	}

	// Probe each distinct impl origin once. Down is the normal state for a
	// design-only session, so it is a warning unless -require-impl.
	origins := m.ImplOrigins()
	var keys []string
	for o := range origins {
		keys = append(keys, o)
	}
	sort.Strings(keys)
	client := &http.Client{Timeout: opts.ImplTimeout}
	for _, origin := range keys {
		surfaces := origins[origin]
		resp, err := client.Get(origin)
		if err != nil {
			sev := Warn
			if opts.RequireImpl {
				sev = Fail
			}
			hint := runHint(m, surfaces)
			report(sev, "impl origin %s DOWN — surfaces: %s%s", origin, strings.Join(surfaces, ", "), hint)
			continue
		}
		resp.Body.Close()
		ok++
		fmt.Fprintf(w, "ok    impl origin %s up — surfaces: %s\n", origin, strings.Join(surfaces, ", "))
	}

	fmt.Fprintf(w, "\nSUMMARY ok=%d warn=%d fail=%d\n", ok, warns, fails)
	if fails > 0 {
		return 1
	}
	return 0
}

// runHint returns the distinct run commands for the given surfaces, if the
// manifest declares any.
func runHint(m *Manifest, surfaces []string) string {
	var runs []string
	seen := map[string]bool{}
	for _, s := range surfaces {
		if meta, okS := m.Surfaces[s]; okS && meta.Run != "" && meta.Run != "design-only" && !seen[meta.Run] {
			seen[meta.Run] = true
			runs = append(runs, meta.Run)
		}
	}
	if len(runs) == 0 {
		return ""
	}
	return " — run: " + strings.Join(runs, " | ")
}

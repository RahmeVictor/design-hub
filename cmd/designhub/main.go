// Command designhub serves a repo's design/ directory for the
// design↔implementation compare harness, with an embedded compare page,
// a manifest preflight, and a screenshot/pixel-diff runner.
//
//	designhub [serve] -dir design [-addr localhost:4400] [-ui dir]
//	designhub doctor  -dir design [-impl-timeout 3s] [-require-impl]
//	designhub shots   -dir design [-out dir] [-hub url] [-chrome path]
//	                  [-only surface[/screen]] [-diff] [-max-diff pct]
//
// Dev-only tooling: never deployed, no auth, binds localhost by default.
// The design dir must hold compare/screens.json — see the README for the schema.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/RahmeVictor/design-hub/internal/hub"
)

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "doctor" || args[0] == "shots") {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", "localhost:4400", "listen address")
		dir := fs.String("dir", "design", "design directory to serve (holds compare/screens.json)")
		ui := fs.String("ui", "", "serve the compare page from <dir>/compare/index.html instead of the embedded one")
		fs.Parse(args)
		log.Fatal(hub.Serve(*addr, *dir, *ui))

	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		dir := fs.String("dir", "design", "design directory (holds compare/screens.json)")
		timeout := fs.Duration("impl-timeout", 3*time.Second, "timeout per implementation-origin probe")
		require := fs.Bool("require-impl", false, "treat unreachable implementation origins as failures")
		fs.Parse(args)
		os.Exit(hub.Doctor(os.Stdout, hub.DoctorOptions{
			Dir: *dir, ImplTimeout: *timeout, RequireImpl: *require,
		}))

	case "shots":
		fs := flag.NewFlagSet("shots", flag.ExitOnError)
		dir := fs.String("dir", "design", "design directory (holds compare/screens.json)")
		out := fs.String("out", "", "output directory (default <dir>/compare/shots)")
		hubURL := fs.String("hub", "http://localhost:4400", "running design hub base URL")
		chrome := fs.String("chrome", "", "chrome binary (default $CHROME, then auto-detect)")
		only := fs.String("only", "", "filter: surface or surface/screen-substring")
		diff := fs.Bool("diff", false, "also produce pixel-diff images and mismatch scores")
		maxDiff := fs.Float64("max-diff", 0, "exit non-zero when any mismatch score exceeds this percent (0 = advisory only)")
		fs.Parse(args)
		os.Exit(hub.Shots(os.Stdout, hub.ShotsOptions{
			Dir: *dir, Out: *out, Hub: *hubURL, Chrome: *chrome,
			Only: *only, Diff: *diff, MaxDiff: *maxDiff,
		}))

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (serve | doctor | shots)\n", cmd)
		os.Exit(2)
	}
}

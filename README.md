# design-hub

Local harness for comparing a committed design package against its live
implementation, screen by screen. One stdlib-only Go binary with three
commands:

- **serve** — hosts a repo's `design/` directory on localhost with an embedded
  side-by-side / overlay compare page at `/compare/`.
- **doctor** — preflights the manifest: schema problems, missing design files,
  unreachable implementation origins.
- **shots** — headless-Chrome screenshot gallery of every screen, optionally
  with pixel-diff images and mismatch scores.

Dev-only tooling: never deployed, no auth, binds localhost by default.

## Install / run

No install needed — run it pinned from any repo that has a design dir:

```
go run github.com/RahmeVictor/design-hub/cmd/designhub@v0.1.0 -dir design
```

Then open <http://localhost:4400/compare/>. A fresh tag can take a few minutes
to appear through proxy.golang.org; `GOPROXY=direct` skips the wait.

```
designhub [serve] -dir design [-addr localhost:4400] [-ui dir]
designhub doctor  -dir design [-impl-timeout 3s] [-require-impl]
designhub shots   -dir design [-out dir] [-hub url] [-chrome path]
                  [-only surface[/screen]] [-diff] [-max-diff pct]
```

The compare page is embedded in the binary and shadows any
`compare/index.html` on disk, so it evolves with the tool instead of drifting
per project (`-ui <dir>` serves a page from disk instead, for hacking on the
UI). Everything else — the manifest, parity notes, shots output — is served
from the design dir.

## The manifest: `design/compare/screens.json`

The design dir must hold `compare/screens.json`. `devices` and `screens` are
required; everything else is optional.

```jsonc
{
  "title": "MyApp — Design ↔ Implementation",    // page + gallery title
  "tokens": ["/prototype/tokens/colors.css"],    // stylesheets the compare page loads,
                                                 //   hub-root-relative — themes the chrome
  "favicon": "/prototype/assets/favicon-32.png", // hub-root-relative
  "parity": "PARITY.md",                         // parity ledger, compare/-relative
  "devices": {                                   // required — viewport per device key
    "phone":   { "width": 430,  "height": 900 },
    "desktop": { "width": 1440, "height": 900 }
  },
  "surfaces": {                                  // sidebar grouping + run hints
    "web": { "label": "Web panel", "run": "make dev" },
    "brand": { "label": "Brand", "run": "design-only" }   // design-only: no run hint shown
  },
  "screens": [
    {
      "surface": "web", "screen": "dashboard", "device": "desktop",
      "designUrl": "/prototype/Dashboard.html",  // hub-root-relative, must exist on disk
      "implUrl": "http://localhost:8000/app/",   // live surface; "" shows a placeholder
      "designNav": "sign in",                    // what to tap inside each frame to reach
      "implNav": "log in as manager"             //   the screen (no deep links assumed)
    }
  ]
}
```

`doctor` validates all of this and probes each distinct `implUrl` origin once.
A down origin is a warning (the normal state for a design-only session) unless
`-require-impl` promotes it to a failure. Exit codes: 0 clean, 1 problems,
2 usage/layout error — same convention for `shots`.

## Compare page

Two modes, toggled in the header and remembered in localStorage:

- **Side by side** — design and implementation iframes scaled to fit.
- **Overlay** — both frames stacked in one stage: an opacity slider for the
  top frame (arrow keys nudge), **Swap** to flip which frame is on top, and a
  **Difference** checkbox (`mix-blend-mode: difference`) that makes 1-pixel
  drift jump out. The top frame receives the clicks, and each iframe scrolls
  independently — overlay compares the initial viewport.

Deep-linking: `#surface/screen` in the URL hash.

## Shots + pixel diff

```
designhub shots -dir design --diff
```

Screenshots every screen's design half (through the running hub) and
implementation half at the device size, writes
`<out>/{slug}-{design,impl,diff}.png`, a gallery `index.html`, and a
machine-readable `report.json` (`scorePercent` per screen). The diff image is
the design dimmed to grayscale with mismatching pixels in magenta; a
per-channel tolerance (default 16/255) absorbs anti-aliasing noise.

Known limits, inherited from the harness model: design pages have no deep
links, so design shots capture landing states only; implementation URLs render
their login screen unless a session was seeded. Treat scores as advisory —
`-max-diff` is off by default.

Chrome is resolved from `-chrome`, `$CHROME`, then auto-detect (macOS app
path, `google-chrome`, `chromium`).

## Framing (CSP)

The compare page iframes your implementation, so the app must allow being
framed by the hub **in dev only**:

```
Content-Security-Policy: frame-ancestors 'self' http://localhost:4400
```

Keep production at `frame-ancestors 'none'`. (Reference implementation:
MOV's `backend/internal/middleware/security.go`.)

## Versioning

Semver tags (`v0.1.0`, …). Pin the tag in your Makefile:

```make
DESIGN_HUB_VERSION ?= v0.1.0
design-hub:
	go run github.com/RahmeVictor/design-hub/cmd/designhub@$(DESIGN_HUB_VERSION) -dir design
```

## License

MIT.

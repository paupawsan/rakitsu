// Package brand holds the Rakitsu terminal brand mark and its accent color,
// shared between the interactive chat TUI's startup banner
// (internal/chat/banner.go) and the CLI's --help banner (cmd/rakitsu), so
// the glyph is generated and maintained in exactly one place.
package brand

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color is Rakitsu's accent blue — the same value baked into the web UI's
// logo mark (web/src/assets/rakitsu-mark.svg) and --accent-agent in
// web/src/styles/theme.css. lipgloss degrades it to the nearest ANSI color
// on terminals without truecolor support.
var Color = lipgloss.Color("#5b8def")

// Mark is the Rakitsu brand mark (docs/assets/brand/rakitsu-mark.svg),
// downsampled to a fixed 34x17 grid of Unicode shade characters (░▒▓█) so its
// actual geometry — not a hand-drawn stand-in — shows up in a terminal,
// which can't rasterize SVG or speak an image protocol (Kitty/Sixel/
// iTerm2).
//
// The mark's "R" and its rocket silhouette are the same interwoven strokes
// by design (a monogram, not two separate shapes laid on top of each
// other) — at this resolution it reads as the Rakitsu emblem, not as an
// instantly-legible letterform, and that's inherent to the source art, not
// a rendering shortcoming. Confirmed by rasterizing the SVG's 14 individual
// path facets and cross-checking each against this rendered mark: 13 of 14
// have ~0% pixel overlap with the real ink, meaning the line art comes from
// thin gaps *between* overlapping facets under the path's fill-rule, not
// from any single facet's own area — there's no clean per-facet split to
// color or font-swap "R" vs "rocket" without re-deriving the stroke
// geometry from scratch.
//
// Generated once, offline, by rasterizing the SVG with rsvg-convert,
// cropping to its ink bounding box, and averaging alpha coverage per cell:
//
//	rsvg-convert -w 640 -a -b none docs/assets/brand/rakitsu-mark.svg -o mark.png
//	go run <coverage-to-shade-grid tool> mark.png 34 17
//
// Re-run that if the mark itself changes; nothing here reads the SVG at
// build or run time.
var Mark = []string{
	`                ▓▓                `,
	`              ▒▓▓▓▓▒              `,
	`            ▒▓░ ▓▒ ░▓▒            `,
	`          ░▓░ ░▒▓▓▒░ ░▓░          `,
	`        ░██▒▒▓▓▒▒▒▒▓▓▒▒█▓░        `,
	`       ▒▓▓▒▒░        ░▒▒▓▓░       `,
	`     ░▓░    ░░▒▒▓▓▒▒░░    ░▓░     `,
	`     ▒▓   ░█░  ▓▒▒▓  ░█░   ▓▒     `,
	`     ▒▓   ░█ ▒▒    ▒▒ █░   ▓▒     `,
	`     ▒▓   ░█▒▒░░  ░░▒▒░    █▒     `,
	`  ░▒▒▓▓   ░█ ░▒▒▒▒▒░░ ░▒▒▒░█▓▒▒░  `,
	`▓▒░  ░▓   ░█▒▒▒░     ░▒▒░░ ▓░   ▒▓`,
	`█░   ░▓░░ ░█ ░▒▒▒░▒▒░    ▒▓▓░   ░█`,
	`█░    ░░▒▓▓█▒        ▒█▒▓▒░░    ░█`,
	`█▒▒▒▒░      ▒▓      ▓▒      ░▒▒▒▒█`,
	`              ▒▓  ▓▒              `,
	`                ▓▓                `,
}

// MarkWidth and MarkHeight are Mark's fixed dimensions, for callers sizing
// layout around it without re-deriving them from the literal.
const (
	MarkWidth  = 34
	MarkHeight = 17
)

// Render returns Mark joined into a single newline-separated string, styled
// in Color. Its rendered dimensions are fixed (MarkWidth x MarkHeight);
// callers compose it into their own layout.
func Render() string {
	return lipgloss.NewStyle().Foreground(Color).Render(strings.Join(Mark, "\n"))
}

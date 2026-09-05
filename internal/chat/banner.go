package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// brandColor is Rakitsu's accent blue — the same value baked into the web
// UI's logo mark (web/src/assets/rakitsu-mark.svg) and --accent-agent in
// web/src/styles/theme.css. lipgloss degrades it to the nearest ANSI color
// on terminals without truecolor support.
var brandColor = lipgloss.Color("#5b8def")

// markGlyph is the Rakitsu brand mark (docs/assets/brand/rakitsu-mark.svg),
// downsampled to a fixed 34x17 grid of Unicode shade characters (░▒▓█) so its
// actual geometry — not a hand-drawn stand-in — shows up in a terminal, which
// can't rasterize SVG or speak an image protocol (Kitty/Sixel/iTerm2).
// Generated once, offline, by rasterizing the SVG with rsvg-convert, cropping
// to its ink bounding box, and averaging alpha coverage per cell:
//
//	rsvg-convert -w 640 -a -b none docs/assets/brand/rakitsu-mark.svg -o mark.png
//	go run <coverage-to-shade-grid tool> mark.png 34 17
//
// Re-run that if the mark itself changes; nothing here reads the SVG at
// build or run time.
var markGlyph = []string{
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

// renderBanner draws the startup banner shown once at the top of a new chat
// session's scrollback: the brand mark beside the version/agent/model
// identity otherwise only visible in the one-line title bar. width is the
// terminal width (Model.width); the banner never exceeds it. Returns "" if
// width is too narrow to fit the (fixed-size) mark without wrapping it.
func renderBanner(width int, version, agentName, modelName string) string {
	const minWidth = 50 // glyph (34 cols) + spacer + a readable sliver of info text
	if width < minWidth {
		return ""
	}

	glyph := lipgloss.NewStyle().Foreground(brandColor).Render(strings.Join(markGlyph, "\n"))

	var versionLine string
	if version != "" {
		versionLine = titleStyle.Render("rakitsu " + version)
	} else {
		versionLine = titleStyle.Render("rakitsu")
	}
	info := strings.Join([]string{
		versionLine,
		"",
		fmt.Sprintf("Agent: %s", agentName),
		fmt.Sprintf("Model: %s", modelName),
	}, "\n")

	// Center: the mark (17 rows) is much taller than the info block (4
	// lines), so the info needs to sit at its vertical middle, not its top.
	body := lipgloss.JoinHorizontal(lipgloss.Center, glyph, "   ", info)

	// Size the box to its content (+2 cols for Padding(0, 1)) but never
	// wider than the terminal allows (-2 cols for the border itself).
	contentWidth := lipgloss.Width(body)
	boxWidth := contentWidth + 2
	if maxWidth := width - 2; boxWidth > maxWidth {
		boxWidth = maxWidth
	}
	if boxWidth < contentWidth {
		boxWidth = contentWidth // never truncate the content itself
	}
	box := borderStyle.Width(boxWidth).Padding(0, 1).Render(body)
	return box + "\n\n"
}

package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	// styleAccent is the focused pane's frame (D55): ANSI blue, so it
	// follows the terminal's theme.
	styleAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	// styleHeader is the header bar (D55).
	styleHeader = lipgloss.NewStyle().Reverse(true)
)

// frame draws lines inside a rounded box width cells wide and height lines
// tall, borders included (D55). The title sits in the top border, cut with
// … when it doesn't fit. A focused box is drawn in the accent colour, the
// others dim. Each line gets one space on the left and is padded or cut to
// the inner width; missing lines are blank, extra ones are dropped.
func frame(title string, lines []string, width, height int, focused bool) []string {
	if width < 4 || height < 2 {
		return nil
	}
	border, head := styleDim, styleTitle
	if focused {
		border, head = styleAccent, styleTitle.Foreground(lipgloss.Color("4"))
	}
	inner := width - 3 // "│ " on the left, "│" on the right
	title = ansi.Truncate(title, width-4, "…")
	top := border.Render("╭ ") + head.Render(title) + border.Render(" "+strings.Repeat("─", max(width-4-ansi.StringWidth(title), 0))+"╮")
	out := []string{top}
	for i := range height - 2 {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, border.Render("│")+" "+pad(l, inner)+border.Render("│"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
}

// within renders s in outer, re-applying outer after every reset inside s,
// so a styled part (a dim suffix, a struck-through or coloured label) no
// longer ends a selected or dimmed line's style early (D58).
func within(outer lipgloss.Style, s string) string {
	probe := outer.Render("x")
	on := probe[:strings.Index(probe, "x")]
	if on == "" {
		return s
	}
	// Built by hand: Lip Gloss restyles a string that already holds escape
	// sequences one character at a time.
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[m")
	return on + strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+on) + "\x1b[m"
}

// cutHints fits a hints line into width cells (D55): when it is too wide,
// whole hints are dropped from the end at a " · " boundary and "…" marks
// the cut; "? help", when the line has it, always stays, at the right end.
func cutHints(hints string, width int) string {
	if ansi.StringWidth(hints) <= width {
		return hints
	}
	parts := strings.Split(hints, " · ")
	help := ""
	if i := slices.Index(parts, "? help"); i >= 0 {
		parts, help = slices.Delete(parts, i, i+1), "? help"
	}
	room := width - ansi.StringWidth(help) - 3 // " …" and at least one space before help
	kept := ""
	for _, p := range parts {
		next := p
		if kept != "" {
			next = kept + " · " + p
		}
		if ansi.StringWidth(next) > room {
			break
		}
		kept = next
	}
	left := strings.TrimLeft(kept+" …", " ")
	if help == "" {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", max(width-ansi.StringWidth(left)-len(help), 1)) + help
}

package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// The console banner: the Latch wordmark, where to open the panel and, on first start, the setup PIN with a link
// that fills it in. Colors are used only in a real terminal (not when the output goes to a file or a service log),
// and NO_COLOR turns them off.

var color = false

func paint(code, s string) string {
	if !color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func rgb(r, g, b int) string { return fmt.Sprintf("38;2;%d;%d;%d", r, g, b) }

var (
	bold   = func(s string) string { return paint("1", s) }
	dim    = func(s string) string { return paint("2", s) }
	green  = func(s string) string { return paint(rgb(74, 222, 128), s) }
	cyan   = func(s string) string { return paint(rgb(103, 232, 249), s) }
	yellow = func(s string) string { return paint(rgb(250, 204, 21), s) }
)

// Letters in the "ANSI Shadow" style, six rows each.
var letters = map[rune][6]string{
	'L': {"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
	'A': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'T': {"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	'C': {" ██████╗", "██╔════╝", "██║     ", "██║     ", "╚██████╗", " ╚═════╝"},
	'H': {"██╗  ██╗", "██║  ██║", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
}

// chrome is a top-to-bottom white-to-gray ramp, like the metal "L." mark.
var chrome = [6][3]int{{255, 255, 255}, {236, 236, 240}, {212, 212, 218}, {176, 176, 184}, {150, 150, 160}, {120, 120, 130}}

func wordmark(version string) string {
	var b strings.Builder
	b.WriteString("\n")
	for row := 0; row < 6; row++ {
		var line strings.Builder
		for _, r := range "LATCH" {
			line.WriteString(letters[r][row])
		}
		c := chrome[row]
		b.WriteString("  " + paint(rgb(c[0], c[1], c[2]), line.String()))
		switch row {
		case 4:
			b.WriteString("  " + bold("ADMIN"))
		case 5:
			b.WriteString("  " + dim("v"+version))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n  " + dim("Web panel for MafiaHub game servers · heylatch.app") + "\n")
	return b.String()
}

// row prints "  label   value" with the label padded and dimmed.
func row(label, value string) string {
	return fmt.Sprintf("    %s %s\n", dim(fmt.Sprintf("%-8s", label)), value)
}

func printBanner(version, dataDir string, links []string, plainHTTP bool) {
	var b strings.Builder
	b.WriteString(wordmark(version))
	b.WriteString("\n  " + green("●") + " " + bold("Running") + "\n")
	for i, u := range links {
		label := "Network"
		if i == 0 {
			label = "Local"
		}
		b.WriteString(row(label, cyan(u)))
	}
	b.WriteString(row("Data", dataDir))
	if plainHTTP {
		b.WriteString("\n  " + yellow("!") + " Plain HTTP: put the panel behind HTTPS (a reverse proxy or --tls-cert)\n")
		b.WriteString("    before signing in over the internet.\n")
	}
	b.WriteString("\n  " + dim("Keep this window open while the panel runs. Ctrl+C stops it and its game servers.") + "\n")
	fmt.Print(b.String())
}

// printSetup shows the PIN in a box with a link that opens the setup page with the PIN filled in.
func printSetup(pin, link string, opened bool) {
	hint := "Open the link to create the owner account."
	if opened {
		hint = "Your browser opens this page. If it doesn't, Ctrl+click the link."
	}
	spaced := pin
	if len(pin) == 6 {
		spaced = pin[:3] + " " + pin[3:]
	}
	lines := [][2]string{
		{bold("First-time setup"), "First-time setup"},
		{"", ""},
		{dim("PIN   ") + bold(spaced), "PIN   " + spaced},
		{dim("Open  ") + cyan(link), "Open  " + link},
		{"", ""},
		{dim(hint), hint},
	}
	width := 0
	for _, l := range lines {
		width = max(width, utf8.RuneCountInString(l[1]))
	}
	var b strings.Builder
	b.WriteString("\n  " + dim("╭"+strings.Repeat("─", width+4)+"╮") + "\n")
	for _, l := range lines {
		pad := width - utf8.RuneCountInString(l[1])
		b.WriteString("  " + dim("│") + "  " + l[0] + strings.Repeat(" ", pad) + "  " + dim("│") + "\n")
	}
	b.WriteString("  " + dim("╰"+strings.Repeat("─", width+4)+"╯") + "\n\n")
	fmt.Print(b.String())
}

// Package ui provides colored logging and confirmation prompts.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	colorRed    = ""
	colorGreen  = ""
	colorYellow = ""
	colorBlue   = ""
	colorBold   = ""
	colorReset  = ""
)

// NoConfirm, when true, makes Confirm always return true (backs --noconfirm/-y).
var NoConfirm bool

// SetColors backs --color=<yes|no|auto>. "auto" follows whether stderr is a tty.
func SetColors(mode string) {
	use := false
	switch mode {
	case "yes":
		use = true
	case "no":
		use = false
	default:
		use = isTTY(os.Stderr)
	}
	if use {
		colorRed, colorGreen, colorYellow, colorBlue, colorBold, colorReset =
			"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[1m", "\x1b[0m"
	} else {
		colorRed, colorGreen, colorYellow, colorBlue, colorBold, colorReset = "", "", "", "", "", ""
	}
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func Info(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s::%s %s\n", colorBlue, colorReset, fmt.Sprintf(format, a...))
}

func Ok(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s::%s %s\n", colorGreen, colorReset, fmt.Sprintf(format, a...))
}

func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s:: warning:%s %s\n", colorYellow, colorReset, fmt.Sprintf(format, a...))
}

// Die prints an error message and exits the process with status 1.
func Die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s:: error:%s %s\n", colorRed, colorReset, fmt.Sprintf(format, a...))
	os.Exit(1)
}

func Bold(s string) string {
	return colorBold + s + colorReset
}

var yesRe = regexp.MustCompile(`^[Yy]$`)

// Confirm shows a y/N prompt, honoring NoConfirm.
func Confirm(format string, a ...any) bool {
	prompt := fmt.Sprintf(format, a...)
	if NoConfirm {
		Info("%s [auto-yes: --noconfirm]", prompt)
		return true
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = trimNewline(line)
	return yesRe.MatchString(line)
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Ask presents a prompt with single-letter choices (the first is the
// default when the user just presses enter) and returns the chosen letter
// in lowercase. Honors NoConfirm by taking the default.
func Ask(prompt string, choices string, format string, a ...any) string {
	return ask(NoConfirm, prompt, choices, format, a...)
}

// AskAlways is Ask, but ignores NoConfirm -- for the one decision that
// isn't a routine confirmation (approving a stranger's build script).
func AskAlways(prompt string, choices string, format string, a ...any) string {
	return ask(false, prompt, choices, format, a...)
}

func ask(autoAnswer bool, prompt string, choices string, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	def := strings.ToLower(string(choices[0]))
	if autoAnswer {
		Info("%s [auto: %s]", msg, def)
		return def
	}
	for {
		fmt.Fprintf(os.Stderr, "%s %s ", msg, prompt)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" {
			return def
		}
		if len(line) == 1 && strings.Contains(strings.ToLower(choices), line) {
			return line
		}
		Warn("please answer with one of: %s", strings.Join(strings.Split(strings.ToLower(choices), ""), "/"))
	}
}

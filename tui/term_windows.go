//go:build windows

package tui

import (
	"golang.org/x/term"
)

func isTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

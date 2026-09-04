// The masked prompt follows Hermes' password handling, at
// ~/Code/hermes-agent/cli.py, where the function to look for is
// _sudo_password_callback, and its terminal side in
// ~/Code/hermes-agent/tools/terminal_tool.py: a secret is typed once, in the
// terminal, and never echoed. Hermes leans on Python's getpass; this reads the
// terminal itself so that it can show an asterisk for each character and put
// the terminal back on every path out.

package vault

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// The keys the prompt understands while a secret is being typed.
const (
	keyInterrupt = 0x03
	keyEndOfFile = 0x04
	keyBackspace = 0x08
	keyDelete    = 0x7f
	keyReturn    = '\r'
	keyNewline   = '\n'
)

// ErrPromptInterrupted means the user pressed the interrupt key instead of
// finishing the secret, so nothing was entered and nothing was kept.
var ErrPromptInterrupted = errors.New("the prompt was stopped before a secret was entered, so nothing was kept")

// AskSecret prints the prompt on the terminal, reads a secret with the
// terminal's own echo turned off, and shows one asterisk for each character
// typed. It refuses anything that is not a terminal, because a pipe cannot hide
// what goes through it, and it puts the terminal back the way it found it on
// every path out, the interrupt key included.
//
// This is the one way a value enters the vault: "/vault add" in the terminal
// and "nerdgenie init" both ask through here.
func AskSecret(terminal *os.File, prompt string) (string, error) {
	settings, err := readTerminalSettings(terminal)
	if err != nil {
		return "", fmt.Errorf("%s is not a terminal, so what you type could not be hidden; enter the secret in the terminal where Nerd Genie is running: %w", terminal.Name(), err)
	}

	quiet := settings
	quiet.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	quiet.Cc[syscall.VMIN] = 1
	quiet.Cc[syscall.VTIME] = 0
	if err := writeTerminalSettings(terminal, quiet); err != nil {
		return "", fmt.Errorf("the terminal %s would not turn its echo off, so the secret was not asked for: %w", terminal.Name(), err)
	}
	defer func() { _ = writeTerminalSettings(terminal, settings) }()

	if _, err := terminal.WriteString(prompt); err != nil {
		return "", fmt.Errorf("the prompt could not be printed on %s: %w", terminal.Name(), err)
	}
	return readMaskedLine(terminal)
}

// readMaskedLine reads one line from the terminal, showing an asterisk for each
// character and never the character itself.
func readMaskedLine(terminal *os.File) (string, error) {
	var typed strings.Builder
	one := make([]byte, 1)
	for {
		read, err := terminal.Read(one)
		if err != nil {
			return "", fmt.Errorf("the terminal %s stopped while the secret was being typed: %w", terminal.Name(), err)
		}
		if read == 0 {
			continue
		}

		switch one[0] {
		case keyReturn, keyNewline:
			_, _ = terminal.WriteString("\r\n")
			return typed.String(), nil
		case keyInterrupt, keyEndOfFile:
			_, _ = terminal.WriteString("\r\n")
			return "", ErrPromptInterrupted
		case keyBackspace, keyDelete:
			rubOut(terminal, &typed)
		default:
			typed.WriteByte(one[0])
			_, _ = terminal.WriteString("*")
		}

		if typed.Len() > MaxValueLength {
			_, _ = terminal.WriteString("\r\n")
			return "", fmt.Errorf("what was typed is longer than %d characters, so it is not a secret and none was kept", MaxValueLength)
		}
	}
}

// rubOut takes the last character off the secret and off the screen. Nothing
// happens when there is nothing to take off.
func rubOut(terminal *os.File, typed *strings.Builder) {
	if typed.Len() == 0 {
		return
	}
	kept := typed.String()
	typed.Reset()
	typed.WriteString(kept[:len(kept)-1])
	_, _ = terminal.WriteString("\b \b")
}

// readTerminalSettings reads a terminal's settings, and fails on anything that
// is not a terminal, which is how the prompt refuses a pipe.
func readTerminalSettings(terminal *os.File) (syscall.Termios, error) {
	var settings syscall.Termios
	if err := terminalControl(terminal, syscall.TCGETS, unsafe.Pointer(&settings)); err != nil {
		return settings, err
	}
	return settings, nil
}

// writeTerminalSettings puts a terminal's settings back or changes them.
func writeTerminalSettings(terminal *os.File, settings syscall.Termios) error {
	return terminalControl(terminal, syscall.TCSETS, unsafe.Pointer(&settings))
}

// terminalControl asks the kernel about a file, which is how a program reads
// and sets the settings of the terminal it is attached to.
func terminalControl(terminal *os.File, request uintptr, settings unsafe.Pointer) error {
	_, _, failure := syscall.Syscall(syscall.SYS_IOCTL, terminal.Fd(), request, uintptr(settings))
	if failure != 0 {
		return failure
	}
	return nil
}

package vault_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/JaredTate/coeus/internal/vault"
)

// pseudoTerminal is a pair of files that behave exactly like a terminal and the
// person sitting at it: what the test writes to the controller arrives as
// typing, and what the program prints on the terminal comes back out of the
// controller.
type pseudoTerminal struct {
	controller *os.File
	terminal   *os.File
}

// openPseudoTerminal opens the pair with the standard library alone.
func openPseudoTerminal(t *testing.T) *pseudoTerminal {
	t.Helper()
	controller, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening /dev/ptmx to make a terminal for the test failed: %v", err)
	}

	unlocked := int32(0)
	if err := deviceControl(controller, syscall.TIOCSPTLCK, unsafe.Pointer(&unlocked)); err != nil {
		t.Fatalf("unlocking the pseudo-terminal failed: %v", err)
	}
	number := uint32(0)
	if err := deviceControl(controller, syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		t.Fatalf("asking for the pseudo-terminal number failed: %v", err)
	}

	terminal, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("opening the terminal side of the pair failed: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = controller.Close()
	})
	return &pseudoTerminal{controller: controller, terminal: terminal}
}

// deviceControl asks the kernel a question about a file, which is how a program
// reads and sets the settings of a terminal.
func deviceControl(file *os.File, request uintptr, argument unsafe.Pointer) error {
	_, _, failure := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), request, uintptr(argument))
	if failure != 0 {
		return failure
	}
	return nil
}

// terminalFlags returns the settings of a terminal, so that a test can say
// whether the prompt put them back the way it found them.
func terminalFlags(t *testing.T, file *os.File) syscall.Termios {
	t.Helper()
	var settings syscall.Termios
	if err := deviceControl(file, syscall.TCGETS, unsafe.Pointer(&settings)); err != nil {
		t.Fatalf("reading the terminal settings failed: %v", err)
	}
	return settings
}

// readExactly reads a fixed number of bytes, which is how the test waits for
// the prompt without waiting on a clock.
func readExactly(t *testing.T, from *os.File, count int) string {
	t.Helper()
	written := make([]byte, count)
	if _, err := io.ReadFull(from, written); err != nil {
		t.Fatalf("reading %d bytes of what the program printed failed: %v", count, err)
	}
	return string(written)
}

// readWhateverIsLeft reads everything the program printed until nothing more
// arrives for a moment.
func readWhateverIsLeft(t *testing.T, from *os.File) string {
	t.Helper()
	if err := from.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("putting a deadline on the terminal read failed: %v", err)
	}
	var seen strings.Builder
	chunk := make([]byte, 256)
	for {
		read, err := from.Read(chunk)
		seen.Write(chunk[:read])
		if err != nil {
			return seen.String()
		}
	}
}

// answer is what one run of the masked prompt gave back.
type answer struct {
	secret string
	err    error
}

// askInTheBackground runs the prompt on the terminal side of a pair while the
// test plays the person typing.
func askInTheBackground(pair *pseudoTerminal, prompt string) <-chan answer {
	answers := make(chan answer, 1)
	go func() {
		secret, err := vault.AskSecret(pair.terminal, prompt)
		answers <- answer{secret: secret, err: err}
	}()
	return answers
}

func TestTheMaskedPromptPrintsOnlyAsterisksAndNeverTheSecret(t *testing.T) {
	pair := openPseudoTerminal(t)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	if printed := readExactly(t, pair.controller, len(prompt)); printed != prompt {
		t.Fatalf("the prompt printed as %q, want %q", printed, prompt)
	}
	if _, err := pair.controller.WriteString("hunter2\n"); err != nil {
		t.Fatalf("typing the secret failed: %v", err)
	}

	got := <-answers
	if got.err != nil {
		t.Fatalf("the masked prompt failed: %v", got.err)
	}
	if got.secret != "hunter2" {
		t.Errorf("the masked prompt gave back %q, want %q", got.secret, "hunter2")
	}

	echoed := readWhateverIsLeft(t, pair.controller)
	if strings.Contains(echoed, "hunter2") {
		t.Fatalf("the terminal echoed the secret itself in %q", echoed)
	}
	if strings.Count(echoed, "*") != len("hunter2") {
		t.Errorf("the terminal showed %q, want one asterisk for each of the %d characters", echoed, len("hunter2"))
	}
	for _, shown := range echoed {
		if shown != '*' && shown != '\r' && shown != '\n' {
			t.Errorf("the terminal showed %q, which holds more than asterisks and the end of the line", echoed)
			break
		}
	}
}

func TestTheMaskedPromptRefusesAPipeBecauseItCannotHideTyping(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("making a pipe failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
		_ = writer.Close()
	}()
	if _, err := writer.WriteString("hunter2\n"); err != nil {
		t.Fatalf("writing into the pipe failed: %v", err)
	}

	secret, err := vault.AskSecret(reader, "Password: ")
	if err == nil {
		t.Fatalf("the masked prompt read a secret from a pipe, where it cannot hide what is typed")
	}
	if secret != "" {
		t.Errorf("the masked prompt gave back %q from a pipe", secret)
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("the error %q does not say that this is not a terminal", err)
	}
}

func TestTheMaskedPromptPutsTheTerminalBackAfterAnInterrupt(t *testing.T) {
	pair := openPseudoTerminal(t)
	before := terminalFlags(t, pair.terminal)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	if printed := readExactly(t, pair.controller, len(prompt)); printed != prompt {
		t.Fatalf("the prompt printed as %q, want %q", printed, prompt)
	}
	if _, err := pair.controller.WriteString("ab\x03"); err != nil {
		t.Fatalf("typing the interrupt failed: %v", err)
	}

	got := <-answers
	if got.err == nil {
		t.Fatalf("the masked prompt answered after an interrupt, and it must give nothing back")
	}
	if got.secret != "" {
		t.Errorf("the masked prompt gave back %q after an interrupt", got.secret)
	}

	after := terminalFlags(t, pair.terminal)
	if after.Lflag != before.Lflag {
		t.Errorf("the terminal was left with the settings %#x, want the %#x it had before the prompt", after.Lflag, before.Lflag)
	}
	if after.Lflag&syscall.ECHO == 0 {
		t.Errorf("the terminal was left with its echo off, so the user would type blind from now on")
	}
}

func TestTheMaskedPromptPutsTheTerminalBackAfterAnAnswer(t *testing.T) {
	pair := openPseudoTerminal(t)
	before := terminalFlags(t, pair.terminal)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	readExactly(t, pair.controller, len(prompt))
	if _, err := pair.controller.WriteString("hunter2\r"); err != nil {
		t.Fatalf("typing the secret failed: %v", err)
	}
	if got := <-answers; got.err != nil || got.secret != "hunter2" {
		t.Fatalf("the masked prompt gave back %q and %v, want hunter2 and no error", got.secret, got.err)
	}

	after := terminalFlags(t, pair.terminal)
	if after.Lflag != before.Lflag {
		t.Errorf("the terminal was left with the settings %#x, want the %#x it had before the prompt", after.Lflag, before.Lflag)
	}
}

func TestBackspaceRubsOutTheLastCharacterOfTheSecret(t *testing.T) {
	pair := openPseudoTerminal(t)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	readExactly(t, pair.controller, len(prompt))
	if _, err := pair.controller.WriteString("hunterX\x7f2\n"); err != nil {
		t.Fatalf("typing the secret with a backspace failed: %v", err)
	}

	got := <-answers
	if got.err != nil {
		t.Fatalf("the masked prompt failed: %v", got.err)
	}
	if got.secret != "hunter2" {
		t.Errorf("the masked prompt gave back %q, want hunter2 after the backspace", got.secret)
	}
}

func TestBackspaceOnAnEmptySecretDoesNothing(t *testing.T) {
	pair := openPseudoTerminal(t)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	readExactly(t, pair.controller, len(prompt))
	if _, err := pair.controller.WriteString("\x7f\x08ok\n"); err != nil {
		t.Fatalf("typing the backspaces failed: %v", err)
	}

	got := <-answers
	if got.err != nil {
		t.Fatalf("the masked prompt failed: %v", got.err)
	}
	if got.secret != "ok" {
		t.Errorf("the masked prompt gave back %q, want ok", got.secret)
	}
}

func TestASecretLongerThanTheCapIsRefused(t *testing.T) {
	pair := openPseudoTerminal(t)
	const prompt = "Password: "
	answers := askInTheBackground(pair, prompt)

	readExactly(t, pair.controller, len(prompt))
	go func() {
		_, _ = pair.controller.WriteString(strings.Repeat("a", vault.MaxValueLength+16) + "\n")
	}()

	got := <-answers
	if got.err == nil {
		t.Fatalf("the masked prompt accepted a secret longer than the cap of %d characters", vault.MaxValueLength)
	}
	if got.secret != "" {
		t.Errorf("the masked prompt gave back a secret that was over the cap")
	}
}

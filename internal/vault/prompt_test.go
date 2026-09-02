package vault_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/JaredTate/coeus/internal/vault"
)

// waitLimit is how long a test waits for the prompt to print something before
// it gives up, because every wait in Coeus has a limit.
const waitLimit = 5 * time.Second

// pseudoTerminal is a pair of files that behave exactly like a terminal and the
// person sitting at it: what the test writes to the controller arrives as
// typing, and everything the program prints on the terminal is collected as it
// comes out of the controller.
type pseudoTerminal struct {
	controller *os.File
	terminal   *os.File
	guard      sync.Mutex
	seen       []byte
}

// openPseudoTerminal opens the pair with the standard library alone, turns off
// the terminal's own rewriting of the output so that a test can read exactly
// what the program printed, and starts collecting that output.
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

	pair := &pseudoTerminal{controller: controller, terminal: terminal}
	settings := terminalFlags(t, terminal)
	settings.Oflag &^= syscall.OPOST
	if err := deviceControl(terminal, syscall.TCSETS, unsafe.Pointer(&settings)); err != nil {
		t.Fatalf("turning off the terminal's rewriting of the output failed: %v", err)
	}
	go pair.collect()
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = controller.Close()
	})
	return pair
}

// collect reads everything the program prints until the terminal side is
// closed, so that no write of the program's can ever block on a full buffer.
func (pair *pseudoTerminal) collect() {
	chunk := make([]byte, 4096)
	for {
		read, err := pair.controller.Read(chunk)
		pair.guard.Lock()
		pair.seen = append(pair.seen, chunk[:read]...)
		pair.guard.Unlock()
		if err != nil {
			return
		}
	}
}

// printed is everything the program has printed on the terminal so far.
func (pair *pseudoTerminal) printed() string {
	pair.guard.Lock()
	defer pair.guard.Unlock()
	return string(pair.seen)
}

// waitForPrinted waits until the program has printed the text, which is how a
// test knows the prompt is up without guessing at how long that takes.
func (pair *pseudoTerminal) waitForPrinted(t *testing.T, wanted string) {
	t.Helper()
	giveUpAt := time.Now().Add(waitLimit)
	for time.Now().Before(giveUpAt) {
		if strings.Contains(pair.printed(), wanted) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the terminal never printed %q; it printed %q", wanted, pair.printed())
}

// typeOnTheKeyboard sends what a person typed into the terminal.
func (pair *pseudoTerminal) typeOnTheKeyboard(t *testing.T, typed string) {
	t.Helper()
	if _, err := pair.controller.WriteString(typed); err != nil {
		t.Fatalf("typing %q into the terminal failed: %v", typed, err)
	}
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

const testPrompt = "Password: "

func TestTheMaskedPromptPrintsOnlyAsterisksAndNeverTheSecret(t *testing.T) {
	pair := openPseudoTerminal(t)
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
	pair.typeOnTheKeyboard(t, "hunter2\n")

	got := <-answers
	if got.err != nil {
		t.Fatalf("the masked prompt failed: %v", got.err)
	}
	if got.secret != "hunter2" {
		t.Errorf("the masked prompt gave back %q, want hunter2", got.secret)
	}

	pair.waitForPrinted(t, testPrompt+strings.Repeat("*", len("hunter2"))+"\r\n")
	echoed := strings.TrimPrefix(pair.printed(), testPrompt)
	if strings.Contains(echoed, "hunter2") {
		t.Fatalf("the terminal echoed the secret itself in %q", echoed)
	}
	if strings.Count(echoed, "*") != len("hunter2") {
		t.Errorf("the terminal showed %q, want one asterisk for each of the %d characters", echoed, len("hunter2"))
	}
	for _, shown := range echoed {
		if shown != '*' && shown != '\r' && shown != '\n' {
			t.Fatalf("the terminal showed %q, which holds more than asterisks and the end of the line", echoed)
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

	secret, err := vault.AskSecret(reader, testPrompt)
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
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
	pair.typeOnTheKeyboard(t, "ab\x03")

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
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
	pair.typeOnTheKeyboard(t, "hunter2\r")

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
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
	pair.typeOnTheKeyboard(t, "hunterX\x7f2\n")

	got := <-answers
	if got.err != nil {
		t.Fatalf("the masked prompt failed: %v", got.err)
	}
	if got.secret != "hunter2" {
		t.Errorf("the masked prompt gave back %q, want hunter2 after the backspace", got.secret)
	}
	pair.waitForPrinted(t, "\b \b")
}

func TestBackspaceOnAnEmptySecretDoesNothing(t *testing.T) {
	pair := openPseudoTerminal(t)
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
	pair.typeOnTheKeyboard(t, "\x7f\x08ok\n")

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
	answers := askInTheBackground(pair, testPrompt)
	pair.waitForPrinted(t, testPrompt)
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

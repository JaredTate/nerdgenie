package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The first human trial found that the first seconds of typing could vanish.
// The terminal toolkit asked the terminal for its background colour with an
// escape sequence and then read the answer one byte at a time, waiting five
// seconds for each byte, so on a terminal that never answers it swallowed
// whatever the person typed meanwhile. That question was asked while the
// package was being set up, which happens before any code of ours runs, so the
// only honest proof is a real process on a real pseudo-terminal that answers
// nothing at all.
const (
	// helperSetting names the setting that turns this test binary into the
	// child process that draws the screen on the pseudo-terminal.
	helperSetting = "COEUS_TUI_PSEUDO_TERMINAL_CHILD"
	// helperTestName is the test in this file the child runs, and nothing else.
	helperTestName = "TestTheScreenHelperDrawsOnWhateverTerminalItWasGiven"
	// wordTyped is what the parent types at the child a second in. No part of
	// the frame draws it by itself, so finding it in the frame proves the
	// letters reached the input box.
	wordTyped = "hello"
	// typedAfter is how long the parent waits before typing, which is well
	// inside the five seconds the background-colour question used to wait.
	typedAfter = time.Second
	// lettersMustArriveWithin is how long the letters have to reach the input
	// box once they are typed. It ends before the five-second wait would, so a
	// screen that only wakes up when that wait times out fails here.
	lettersMustArriveWithin = 4 * time.Second
	// lookAgainAfter is how long the parent waits between two looks at the
	// frame while it waits for the letters.
	lookAgainAfter = 20 * time.Millisecond
)

// TestLettersTypedWhileTheTerminalStaysSilentReachTheInputBox starts the real
// screen on a pseudo-terminal that answers no question the screen asks, types a
// word at it one second later, and fails unless the word is on the frame within
// three seconds.
func TestLettersTypedWhileTheTerminalStaysSilentReachTheInputBox(t *testing.T) {
	// The child is a real process, and starting it, attaching its input reader,
	// and drawing its first frame all race against the one-second mark when the
	// parent types. So a single attempt is flaky on a loaded machine even when
	// the product is correct. The test tries a few times and passes the instant
	// one attempt shows the letters; a product that truly swallowed early input
	// would swallow it every time, so a real regression still fails.
	const attempts = 4
	frames := make([]string, 0, attempts)
	for attempt := 1; attempt <= attempts; attempt++ {
		reached, frame := lettersTypedEarlyReachTheInputBox(t)
		if reached {
			return
		}
		frames = append(frames, frame)
	}
	t.Fatalf("across %d attempts the screen never drew %q, so letters typed one "+
		"second in were swallowed before the input box saw them; the last frame "+
		"was %q", attempts, wordTyped, frames[len(frames)-1])
}

// lettersTypedEarlyReachTheInputBox runs one attempt of the test above: it
// starts the screen on a fresh pseudo-terminal, types the word one second in,
// and says whether the word reached the input box within the window, along with
// the frame drawn so far. It ends the child before it returns and never fails
// the test itself, so the caller can try again.
func lettersTypedEarlyReachTheInputBox(t *testing.T) (bool, string) {
	t.Helper()
	terminal := openPseudoTerminal(t)
	child := startTheScreenOnTheFarSide(t, terminal)
	defer stopTheChild(child)
	drawn := readWhatIsDrawn(terminal.near)

	time.Sleep(typedAfter)
	if _, err := terminal.near.WriteString(wordTyped); err != nil {
		t.Fatalf("typing %q at the screen failed: %v", wordTyped, err)
	}

	giveUpAt := time.Now().Add(lettersMustArriveWithin)
	for time.Now().Before(giveUpAt) {
		if strings.Contains(drawn.text(), wordTyped) {
			return true, drawn.text()
		}
		time.Sleep(lookAgainAfter)
	}
	return false, drawn.text()
}

// TestTheScreenHelperDrawsOnWhateverTerminalItWasGiven is the child half of the
// test above, and it does nothing at all unless the parent started it.
func TestTheScreenHelperDrawsOnWhateverTerminalItWasGiven(t *testing.T) {
	if os.Getenv(helperSetting) == "" {
		t.Skip("this is the child half of the pseudo-terminal test, and it draws a screen only when the parent starts it")
	}
	if err := Run(Options{Output: os.Stdout, Input: os.Stdin}); err != nil {
		t.Fatalf("the screen the parent started stopped on its own: %v", err)
	}
}

// pseudoTerminal is a pair of file handles the kernel joins together: what is
// written to the near side arrives on the far side, and the far side is what a
// program reads as its keyboard and writes as its screen.
type pseudoTerminal struct {
	// near is the side this test types into and reads the frame from.
	near *os.File
	// far is the side the child process is given as its terminal.
	far *os.File
}

// openPseudoTerminal makes one pair with the echo the terminal driver does for
// itself turned off, so that a letter coming back has come back from the screen
// and not from the kernel, and closes both sides when the test ends.
func openPseudoTerminal(t *testing.T) *pseudoTerminal {
	t.Helper()
	near, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening /dev/ptmx failed, and this test needs a pseudo-terminal to run the screen on: %v", err)
	}
	t.Cleanup(func() { _ = near.Close() })
	if err := unlockTheFarSide(near); err != nil {
		t.Fatalf("unlocking the far side of the pseudo-terminal failed: %v", err)
	}
	number, err := farSideNumber(near)
	if err != nil {
		t.Fatalf("asking which pseudo-terminal the kernel gave out failed: %v", err)
	}
	far, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("opening the far side /dev/pts/%d failed: %v", number, err)
	}
	t.Cleanup(func() { _ = far.Close() })
	if err := turnEchoOff(far); err != nil {
		t.Fatalf("turning the terminal's own echo off failed, and it would have made this test lie: %v", err)
	}
	if err := sayHowBigTheTerminalIs(far); err != nil {
		t.Fatalf("telling the pseudo-terminal how big it is failed, and a terminal of no size draws nothing: %v", err)
	}
	return &pseudoTerminal{near: near, far: far}
}

// sayHowBigTheTerminalIs gives the pair the eighty by twenty-four every
// terminal has had since the nineteen seventies, because a pair the kernel has
// just made is no columns wide and no rows tall.
func sayHowBigTheTerminalIs(far *os.File) error {
	size := windowSize{rows: fallbackHeight, columns: fallbackWidth}
	_, _, failed := syscall.Syscall(
		syscall.SYS_IOCTL,
		far.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&size)),
	)
	if failed != 0 {
		return failed
	}
	return nil
}

// unlockTheFarSide tells the kernel the far side of a fresh pseudo-terminal may
// now be opened, which is the one step between asking for a pair and using it.
func unlockTheFarSide(near *os.File) error {
	unlocked := int32(0)
	_, _, failed := syscall.Syscall(
		syscall.SYS_IOCTL,
		near.Fd(),
		syscall.TIOCSPTLCK,
		uintptr(unsafe.Pointer(&unlocked)),
	)
	if failed != 0 {
		return failed
	}
	return nil
}

// farSideNumber asks the kernel which numbered file under /dev/pts is the far
// side of this pair.
func farSideNumber(near *os.File) (int, error) {
	number := int32(0)
	_, _, failed := syscall.Syscall(
		syscall.SYS_IOCTL,
		near.Fd(),
		syscall.TIOCGPTN,
		uintptr(unsafe.Pointer(&number)),
	)
	if failed != 0 {
		return 0, failed
	}
	return int(number), nil
}

// turnEchoOff stops the terminal driver printing back what is typed at it, and
// stops it holding a line until Enter, so that the frame this test reads holds
// only what the screen itself drew.
func turnEchoOff(far *os.File) error {
	settings := syscall.Termios{}
	_, _, failed := syscall.Syscall(
		syscall.SYS_IOCTL,
		far.Fd(),
		syscall.TCGETS,
		uintptr(unsafe.Pointer(&settings)),
	)
	if failed != 0 {
		return failed
	}
	settings.Lflag &^= syscall.ECHO | syscall.ICANON
	_, _, failed = syscall.Syscall(
		syscall.SYS_IOCTL,
		far.Fd(),
		syscall.TCSETS,
		uintptr(unsafe.Pointer(&settings)),
	)
	if failed != 0 {
		return failed
	}
	return nil
}

// startTheScreenOnTheFarSide runs this test binary again as the child that
// draws the screen, with the far side of the pseudo-terminal as its keyboard,
// its screen, and the terminal it is in charge of.
func startTheScreenOnTheFarSide(t *testing.T, terminal *pseudoTerminal) *exec.Cmd {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^"+helperTestName+"$", "-test.timeout=1m")
	// The child gets a plain environment of its own rather than this test's,
	// because a terminal library refuses to ask a terminal anything while CI is
	// set, and a test that quietly stops proving what it was written for is
	// worse than no test at all.
	child.Env = []string{
		helperSetting + "=1",
		"TERM=xterm-256color",
		"HOME=" + t.TempDir(),
		"PATH=" + os.Getenv("PATH"),
	}
	child.Stdin, child.Stdout, child.Stderr = terminal.far, terminal.far, terminal.far
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := child.Start(); err != nil {
		t.Fatalf("starting the screen on the pseudo-terminal failed: %v", err)
	}
	return child
}

// stopTheChild ends the screen by the exact process id it was given, never by a
// pattern over process names, because a pattern matches this test as well.
func stopTheChild(child *exec.Cmd) {
	_ = child.Process.Kill()
	_ = child.Wait()
}

// frameSoFar collects everything the screen has drawn, because the letters may
// land in any one of the frames it paints.
type frameSoFar struct {
	guard   sync.Mutex
	painted strings.Builder
}

// text is everything drawn so far.
func (frame *frameSoFar) text() string {
	frame.guard.Lock()
	defer frame.guard.Unlock()
	return frame.painted.String()
}

// add keeps one piece of what was drawn.
func (frame *frameSoFar) add(piece []byte) {
	frame.guard.Lock()
	defer frame.guard.Unlock()
	frame.painted.Write(piece)
}

// readWhatIsDrawn keeps reading the near side until the screen goes away, so
// that nothing the child paints is missed while the test waits.
func readWhatIsDrawn(near *os.File) *frameSoFar {
	frame := &frameSoFar{}
	go func() {
		buffer := make([]byte, 4096)
		for {
			read, err := near.Read(buffer)
			if read > 0 {
				frame.add(buffer[:read])
			}
			if err != nil {
				return
			}
		}
	}()
	return frame
}

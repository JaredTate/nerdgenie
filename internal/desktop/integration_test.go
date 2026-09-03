//go:build integration

package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// fixtureTitle is what the fixture window is called. The worker finds the
// window by this title, so nothing the user opened is ever touched.
const fixtureTitle = "Coeus desktop integration fixture"

// workerCommand is node and the built worker, or a reason it cannot be run here.
//
// The first check is the one that matters. These tests open a window on the
// screen of whoever runs them and drive it through the accessibility tree, and
// on a machine somebody is logged in to that can wake the screen reader. So they
// run only when COEUS_LIVE_DESKTOP asks for them, which is the same gate the
// worker's own fixture-window suite keeps.
func workerCommand(t *testing.T) []string {
	t.Helper()
	if os.Getenv(liveDesktopSwitch) != "1" {
		t.Skipf("these tests drive this machine's own screen, so they run only when %s=1 asks for them", liveDesktopSwitch)
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("finding the working folder failed: %v", err)
	}
	script := filepath.Join(filepath.Dir(filepath.Dir(here)), "worker", "desktop", "dist", "main.js")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("the desktop worker is not built at %s, so run: npm --prefix worker/desktop install && npm --prefix worker/desktop run build", script)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on the PATH, so the desktop worker cannot be started")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("there is no display on this machine, so there is no desktop to drive")
	}
	if _, err := exec.LookPath("zenity"); err != nil {
		t.Skip("zenity is not installed, so there is no fixture window to open")
	}
	return []string{node, script, "--pacing", "fast"}
}

// openFixtureWindow opens the one window these tests act in and closes it after.
func openFixtureWindow(t *testing.T) *bytes.Buffer {
	t.Helper()
	printed := &bytes.Buffer{}
	window := exec.Command("zenity", "--entry", "--title="+fixtureTitle, "--text=Type here")
	window.Env = append(os.Environ(), "GDK_BACKEND=x11")
	window.Stdout = printed
	if err := window.Start(); err != nil {
		t.Fatalf("opening the fixture window failed: %v", err)
	}
	t.Cleanup(func() {
		if window.Process != nil {
			_ = window.Process.Kill()
		}
		_ = window.Wait()
	})
	time.Sleep(2500 * time.Millisecond)
	return printed
}

// newRealDesktop builds the Go side over the real worker started from command.
func newRealDesktop(t *testing.T, command []string) *Desktop {
	t.Helper()
	start, err := ProcessStart(command, func(format string, arguments ...any) { t.Logf(format, arguments...) })
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	desktop, err := New(Options{
		Start:      start,
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
		Note:       func(format string, arguments ...any) { t.Logf(format, arguments...) },
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}
	t.Cleanup(func() {
		if err := desktop.Close(); err != nil {
			t.Errorf("closing the desktop failed: %v", err)
		}
	})
	return desktop
}

// openedFixture opens the fixture window through the worker and returns the
// numbers of the text box and the OK button on it.
func openedFixture(ctx context.Context, t *testing.T, desktop *Desktop) (int, int) {
	t.Helper()
	if err := desktop.Launch(ctx, fixtureTitle, "a window with a text box opens"); err != nil {
		t.Fatalf("opening the fixture window through the worker failed: %v", err)
	}
	picture, err := desktop.Screenshot(ctx)
	if err != nil {
		t.Fatalf("taking a screenshot of the fixture window failed: %v", err)
	}
	if len(picture.PNGBase64) < 1000 {
		t.Errorf("the picture is %d characters of base64, want a real one", len(picture.PNGBase64))
	}
	box, ok := numberOf(picture.Marks, "Type here")
	confirm, alsoOK := numberOf(picture.Marks, "OK")
	if !ok || !alsoOK {
		t.Fatalf("the marks are %+v, want the text box and the OK button of the fixture window", picture.Marks)
	}
	return box, confirm
}

func TestTheRealWorkerDrivesAFixtureWindowOnThisMachine(t *testing.T) {
	command := workerCommand(t)
	printed := openFixtureWindow(t)
	desktop := newRealDesktop(t, command)
	ctx, done := context.WithTimeout(context.Background(), 3*time.Minute)
	defer done()

	health, err := desktop.Health(ctx)
	if err != nil {
		t.Fatalf("asking the real worker whether it is healthy failed: %v", err)
	}
	if !health.Healthy || health.DriverVersion == "" {
		t.Fatalf("the worker reports %+v, want a healthy worker naming its driver", health)
	}

	box, confirm := openedFixture(ctx, t, desktop)
	if err := desktop.Click(ctx, box, "the text box takes the typing"); err != nil {
		t.Fatalf("clicking the text box failed: %v", err)
	}
	if err := desktop.Type(ctx, "nine years of DigiByte", "the text box holds the post"); err != nil {
		t.Fatalf("typing into the text box failed: %v", err)
	}
	if err := desktop.Press(ctx, "ctrl+a", "the text is selected"); err != nil && !strings.Contains(err.Error(), "did not happen") {
		t.Fatalf("pressing a key combination failed: %v", err)
	}

	// Clicking OK closes the window, so zenity prints what was typed into it and
	// the worker's next reading finds nothing. Either answer is fine; what the
	// fixture printed is the proof.
	_ = desktop.Click(ctx, confirm, "the dialog closes")
	time.Sleep(1500 * time.Millisecond)
	if got := strings.TrimSpace(printed.String()); got != "nine years of DigiByte" {
		t.Errorf("the fixture window reported %q, want exactly what the worker typed into it", got)
	}
}

func TestTheRealWorkerPhotographsTheWholeScreenBeforeAnythingIsLaunched(t *testing.T) {
	command := workerCommand(t)
	openFixtureWindow(t)
	desktop := newRealDesktop(t, command)
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()

	picture, err := desktop.Screenshot(ctx)

	if err != nil {
		t.Fatalf("taking a screenshot with nothing launched failed: %v, and looking at the screen needs no application", err)
	}
	if !isPNG(picture.PNGBase64) {
		t.Errorf("the picture is %d characters of base64 and does not begin like a PNG, want a real picture of the screen", len(picture.PNGBase64))
	}
	if picture.Application != "" || len(picture.Marks) != 0 {
		t.Errorf("the picture is of %q with %d controls numbered, want the whole screen with none numbered, because no application is granted",
			picture.Application, len(picture.Marks))
	}
	if !slices.Contains(picture.Windows, fixtureTitle) {
		t.Errorf("the windows on the screen are %q, want the fixture window %q among them", picture.Windows, fixtureTitle)
	}
}

func TestTheRealWorkerPutsTextOnTheClipboardAndPutsTheUsersOwnBack(t *testing.T) {
	command := workerCommand(t)
	openFixtureWindow(t)
	desktop := newRealDesktop(t, command)
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()

	// The clipboard belongs to the whole machine, so it goes through the same
	// door as every other desktop action and an application has to be open.
	openedFixture(ctx, t, desktop)

	theirs, err := desktop.Clipboard(ctx)
	if err != nil {
		t.Fatalf("reading the machine's clipboard failed: %v", err)
	}
	defer func() {
		if err := desktop.SetClipboard(ctx, theirs); err != nil {
			t.Errorf("putting the user's own clipboard back failed: %v", err)
		}
	}()

	if err := desktop.SetClipboard(ctx, "nine years of DigiByte"); err != nil {
		t.Fatalf("putting text on the clipboard failed: %v", err)
	}
	held, err := desktop.Clipboard(ctx)
	if err != nil {
		t.Fatalf("reading the clipboard back failed: %v", err)
	}
	if held != "nine years of DigiByte" {
		t.Errorf("the clipboard holds %q, want what was put on it", held)
	}
}

// isPNG says whether base64 text begins the way a PNG file does.
func isPNG(pictureBase64 string) bool {
	if len(pictureBase64) < 12 {
		return false
	}
	head, err := base64.StdEncoding.DecodeString(pictureBase64[:12])
	return err == nil && len(head) >= 4 && string(head[1:4]) == "PNG"
}

// numberOf is the number of the mark with that label, when there is one.
func numberOf(marks []contract.DesktopMark, name string) (int, bool) {
	for _, one := range marks {
		if one.Name == name {
			return one.Number, true
		}
	}
	return 0, false
}

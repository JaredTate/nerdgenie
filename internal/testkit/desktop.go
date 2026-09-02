package testkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// FakeDesktop is a desktop nobody can see: it refuses an application the user
// has not granted, records every action, and shows one fixture window with three
// numbered controls on it.
type FakeDesktop struct {
	guard     sync.Mutex
	granted   map[string]bool
	running   string
	actions   []string
	clipboard string
}

// NewFakeDesktop returns a desktop with nothing granted and nothing running.
func NewFakeDesktop() *FakeDesktop {
	return &FakeDesktop{granted: map[string]bool{}}
}

// Grant is the user saying yes to one application for this session.
func (desktop *FakeDesktop) Grant(application string) {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	desktop.granted[application] = true
}

// Actions is everything the desktop was asked to do, in order.
func (desktop *FakeDesktop) Actions() []string {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	copied := make([]string, len(desktop.actions))
	copy(copied, desktop.actions)
	return copied
}

// Launch opens an application, or refuses one the user has not granted.
func (desktop *FakeDesktop) Launch(_ context.Context, application string) error {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if !desktop.granted[application] {
		return fmt.Errorf("the user has not granted %q this session, so ask before opening it", application)
	}
	desktop.running = application
	desktop.actions = append(desktop.actions, "launch "+application)
	return nil
}

// Screenshot returns the fixture window with its controls numbered.
func (desktop *FakeDesktop) Screenshot(_ context.Context) (contract.DesktopScreenshot, error) {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if err := desktop.somethingRunning(); err != nil {
		return contract.DesktopScreenshot{}, err
	}
	return contract.DesktopScreenshot{PNGBase64: fixturePicture, Marks: fixtureDesktopMarks()}, nil
}

// Click clicks the control with that number.
func (desktop *FakeDesktop) Click(_ context.Context, mark int) error {
	return desktop.act(fmt.Sprintf("click %d", mark), mark)
}

// Type types text at human pacing.
func (desktop *FakeDesktop) Type(_ context.Context, text string) error {
	return desktop.act("type "+text, 0)
}

// Press presses a key combination.
func (desktop *FakeDesktop) Press(_ context.Context, keys string) error {
	return desktop.act("press "+keys, 0)
}

// Drag drags from one numbered control to another.
func (desktop *FakeDesktop) Drag(_ context.Context, fromMark int, toMark int) error {
	if err := desktop.act(fmt.Sprintf("drag %d to %d", fromMark, toMark), fromMark); err != nil {
		return err
	}
	return desktop.knownMark(toMark)
}

// Clipboard reads what is on the clipboard.
func (desktop *FakeDesktop) Clipboard(_ context.Context) (string, error) {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if err := desktop.somethingRunning(); err != nil {
		return "", err
	}
	return desktop.clipboard, nil
}

// SetClipboard puts text on the clipboard.
func (desktop *FakeDesktop) SetClipboard(_ context.Context, text string) error {
	if err := desktop.act("set the clipboard", 0); err != nil {
		return err
	}
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	desktop.clipboard = text
	return nil
}

// act records one action, after checking that something is running and that the
// mark, when there is one, is on the screen.
func (desktop *FakeDesktop) act(what string, mark int) error {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if err := desktop.somethingRunning(); err != nil {
		return err
	}
	if mark != 0 {
		if err := desktop.knownMark(mark); err != nil {
			return err
		}
	}
	desktop.actions = append(desktop.actions, what)
	return nil
}

// somethingRunning reports plainly when no application has been opened yet.
func (desktop *FakeDesktop) somethingRunning() error {
	if desktop.running == "" {
		return fmt.Errorf("no application is open on the desktop, so launch one before acting on it")
	}
	return nil
}

// knownMark reports plainly when the number is not on the screen.
func (desktop *FakeDesktop) knownMark(mark int) error {
	for _, known := range fixtureDesktopMarks() {
		if known.Number == mark {
			return nil
		}
	}
	return fmt.Errorf("there is no control numbered %d on the screen, so take a screenshot and use a number from it", mark)
}

// fixtureDesktopMarks is the one window the fake desktop shows.
func fixtureDesktopMarks() []contract.DesktopMark {
	return []contract.DesktopMark{
		{Number: 1, Role: "textbox", Name: "Document"},
		{Number: 2, Role: "button", Name: "Save"},
		{Number: 3, Role: "menu item", Name: "File"},
	}
}

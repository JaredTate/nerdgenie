package desktop

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// Start starts one desktop worker. The running program passes ProcessStart; a
// test passes a worker of its own on a pair of pipes.
type Start func(ctx context.Context) (*Connection, error)

// Options is everything the Go side of the desktop needs.
type Options struct {
	// Start starts one worker.
	Start Start
	// Channel is where the user grants an application and sees a preview.
	Channel contract.Channel
	// Permission rules on the actions that cannot be undone.
	Permission contract.Permission
	// Clock is where this package reads the time.
	Clock contract.Clock
	// Note writes one line about what happened, and may be left out.
	Note func(format string, arguments ...any)
}

// Desktop is the Go side of the desktop worker. It implements contract.Desktop,
// keeps one worker alive, grants an application once per session, and previews
// every action that cannot be undone.
type Desktop struct {
	options    Options
	guard      sync.Mutex
	connection *Connection
	talker     *client
	granted    map[string]bool
	open       string
}

// New builds the desktop, and refuses options with a piece missing.
func New(options Options) (*Desktop, error) {
	switch {
	case options.Start == nil:
		return nil, errors.New("the desktop needs a start function to start its worker with, and none was given")
	case options.Channel == nil:
		return nil, errors.New("the desktop needs a channel to grant an application through, and none was given")
	case options.Permission == nil:
		return nil, errors.New("the desktop needs a permission function to rule on what cannot be undone, and none was given")
	case options.Clock == nil:
		return nil, errors.New("the desktop needs a clock to read the time from, and none was given")
	}
	if options.Note == nil {
		options.Note = func(string, ...any) {}
	}
	return &Desktop{options: options, granted: map[string]bool{}}, nil
}

// Launch opens an application, or brings it forward if it is already open. The
// user grants an application once per session, through a preview on the channel.
func (desktop *Desktop) Launch(ctx context.Context, application string) error {
	return desktop.LaunchExpecting(ctx, application, "")
}

// LaunchExpecting opens an application and checks what the model said it
// expected to see. Launch is this with no expectation.
func (desktop *Desktop) LaunchExpecting(ctx context.Context, application string, expectation string) error {
	if err := desktop.grant(ctx, application); err != nil {
		return err
	}
	var diff diffAnswer
	err := desktop.call(ctx, "launch", map[string]any{"application": application, "expectation": expectation}, &diff)
	if err != nil {
		return err
	}
	desktop.guard.Lock()
	desktop.open = application
	desktop.guard.Unlock()
	desktop.options.Note("the desktop opened the application %q, showing %q", application, diff.Title)
	return unmet(diff, expectation)
}

// Screenshot returns the granted application's window with its controls numbered.
func (desktop *Desktop) Screenshot(ctx context.Context) (contract.DesktopScreenshot, error) {
	if err := desktop.requireOpen(); err != nil {
		return contract.DesktopScreenshot{}, err
	}
	var picture screenshotAnswer
	if err := desktop.call(ctx, "screenshot", map[string]any{}, &picture); err != nil {
		return contract.DesktopScreenshot{}, err
	}
	return contract.DesktopScreenshot{PNGBase64: picture.PNGBase64, Marks: marksOf(picture.Marks)}, nil
}

// Click clicks the control with that number.
func (desktop *Desktop) Click(ctx context.Context, mark int) error {
	return desktop.ClickExpecting(ctx, mark, "")
}

// ClickExpecting clicks the control and checks what the model expected to happen.
func (desktop *Desktop) ClickExpecting(ctx context.Context, mark int, expectation string) error {
	if err := desktop.requireOpen(); err != nil {
		return err
	}
	return desktop.act(ctx, "click", map[string]any{"mark": mark, "expectation": expectation}, expectation)
}

// Type types text at human pacing. Typing into a field cannot be undone, so it
// goes through the permission function first.
func (desktop *Desktop) Type(ctx context.Context, text string) error {
	return desktop.TypeExpecting(ctx, text, "")
}

// TypeExpecting types text and checks what the model expected to happen.
func (desktop *Desktop) TypeExpecting(ctx context.Context, text string, expectation string) error {
	if err := desktop.requireOpen(); err != nil {
		return err
	}
	intent := fmt.Sprintf("type into the application %s on the desktop", desktop.openApplication())
	if err := desktop.permit(ctx, intent, desktop.openApplication(), text); err != nil {
		return err
	}
	return desktop.act(ctx, "type", map[string]any{"text": text, "expectation": expectation}, expectation)
}

// Press presses a key combination, such as "ctrl+s".
func (desktop *Desktop) Press(ctx context.Context, keys string) error {
	return desktop.PressExpecting(ctx, keys, "")
}

// PressExpecting presses a key combination and checks what the model expected.
func (desktop *Desktop) PressExpecting(ctx context.Context, keys string, expectation string) error {
	if err := desktop.requireOpen(); err != nil {
		return err
	}
	return desktop.act(ctx, "press", map[string]any{"keys": keys, "expectation": expectation}, expectation)
}

// Drag drags from one numbered control to another. A drag cannot be undone, so
// it goes through the permission function first.
func (desktop *Desktop) Drag(ctx context.Context, fromMark int, toMark int) error {
	return desktop.DragExpecting(ctx, fromMark, toMark, "")
}

// DragExpecting drags and checks what the model expected to happen.
func (desktop *Desktop) DragExpecting(ctx context.Context, fromMark int, toMark int, expectation string) error {
	if err := desktop.requireOpen(); err != nil {
		return err
	}
	intent := fmt.Sprintf("drag from control %d to control %d in the application %s", fromMark, toMark, desktop.openApplication())
	if err := desktop.permit(ctx, intent, fmt.Sprintf("control %d", fromMark), ""); err != nil {
		return err
	}
	return desktop.act(ctx, "drag", map[string]any{"fromMark": fromMark, "toMark": toMark, "expectation": expectation}, expectation)
}

// Clipboard reads what is on the machine's clipboard.
func (desktop *Desktop) Clipboard(ctx context.Context) (string, error) {
	var held clipboardAnswer
	if err := desktop.call(ctx, "clipboardGet", map[string]any{}, &held); err != nil {
		return "", err
	}
	return held.Text, nil
}

// SetClipboard puts text on the machine's clipboard. Pasting cannot be undone,
// so it goes through the permission function first.
func (desktop *Desktop) SetClipboard(ctx context.Context, text string) error {
	if err := desktop.permit(ctx, "put text on the machine's clipboard, ready to paste", "the clipboard", text); err != nil {
		return err
	}
	return desktop.call(ctx, "clipboardSet", map[string]any{"text": text}, &writtenAnswer{})
}

// Health is what the worker says about whether it can act on this machine.
type Health struct {
	// Healthy is true when the worker can act on the desktop.
	Healthy bool
	// DriverVersion is what the desktop driver calls itself.
	DriverVersion string
	// Display says which display server this desktop runs on.
	Display string
	// Detail says what to fix when the worker is not healthy.
	Detail string
}

// Health says whether the worker can act on the desktop, and what to fix.
func (desktop *Desktop) Health(ctx context.Context) (Health, error) {
	var health healthAnswer
	if err := desktop.call(ctx, "health", map[string]any{}, &health); err != nil {
		return Health{}, err
	}
	desktop.options.Note("the desktop worker reports driver %s on %s", health.DriverVersion, health.Display)
	return Health{
		Healthy:       health.Healthy,
		DriverVersion: health.DriverVersion,
		Display:       health.Display,
		Detail:        health.Detail,
	}, nil
}

// Close stops the worker. Closing a desktop that is already closed is harmless.
func (desktop *Desktop) Close() error {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	return desktop.stopWorker()
}

// act runs one action and turns an expectation that was not met into an error,
// because the model has to be told rather than left to guess.
func (desktop *Desktop) act(ctx context.Context, method string, params map[string]any, expectation string) error {
	var diff diffAnswer
	if err := desktop.call(ctx, method, params, &diff); err != nil {
		return err
	}
	return unmet(diff, expectation)
}

// unmet is the error for an action that ran but did not do what was expected.
func unmet(diff diffAnswer, expectation string) error {
	if diff.ExpectationMet {
		return nil
	}
	if expectation == "" {
		return fmt.Errorf("the action ran but changed nothing on the screen, and %s", said(diff.Seen))
	}
	return fmt.Errorf("the action ran but %q did not happen, and %s", expectation, said(diff.Seen))
}

// said turns the worker's sentence about what changed into part of a sentence.
func said(seen string) string {
	if seen == "" {
		return "the worker said nothing more about it"
	}
	return seen
}

// marksOf turns the protocol's marks into the contract's own.
func marksOf(sent []mark) []contract.DesktopMark {
	marks := make([]contract.DesktopMark, 0, len(sent))
	for _, one := range sent {
		marks = append(marks, contract.DesktopMark{Number: one.Number, Role: one.Role, Name: one.Name})
	}
	return marks
}

// requireOpen refuses to act when no application has been opened yet.
func (desktop *Desktop) requireOpen() error {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if desktop.open == "" {
		return errors.New("no application is open on the desktop, so launch one before acting on it")
	}
	return nil
}

// openApplication is the application that is open, or an empty name.
func (desktop *Desktop) openApplication() string {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	return desktop.open
}

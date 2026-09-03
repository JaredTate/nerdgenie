package desktop

import (
	"context"
	"reflect"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// The act-and-assert rule says every action states what the model expected to
// happen and the worker checks it. It only holds if the expectation is on the
// method the caller actually calls: while the expectation lived on a twin of
// each action, the only caller in the running program called the plain name and
// the expectation the model wrote was thrown away before it left the tool.

// theFiveActions is each action and how many things it carries besides its
// receiver and its context: what to act on, and what the model expected.
var theFiveActions = map[string]int{
	"Launch": 2,
	"Click":  2,
	"Type":   2,
	"Press":  2,
	"Drag":   3,
}

func TestEveryActionCarriesWhatTheModelExpectedAndHasNoTwin(t *testing.T) {
	shape := reflect.TypeOf(&Desktop{})
	for name, carried := range theFiveActions {
		method, found := shape.MethodByName(name)
		if !found {
			t.Errorf("the desktop has no %s at all, and it is one of the five actions", name)
			continue
		}
		if carries := method.Type.NumIn() - 2; carries != carried {
			t.Errorf("the arguments %s takes besides its receiver and its context number %d, want %d: what to act on, and what the model"+
				" expected to happen", name, carries, carried)
			continue
		}
		if last := method.Type.In(method.Type.NumIn() - 1); last.Kind() != reflect.String {
			t.Errorf("the last thing %s carries is a %s, and it must be the expectation, written as a string", name, last)
		}
		if _, twin := shape.MethodByName(name + "Expecting"); twin {
			t.Errorf("%sExpecting is still there, and a twin is how the expectation was lost: the only caller in the running program calls %s",
				name, name)
		}
	}
}

// desktopAsTheContractShouldRead is the shape contract.Desktop takes when the
// lines finding 46 of brief 6.7 asks for are added to it: the same eight
// methods, with each of the five actions carrying what the model expected to
// happen. When those lines land, this declaration goes and the line below names
// contract.Desktop instead.
type desktopAsTheContractShouldRead interface {
	Launch(ctx context.Context, application string, expectation string) error
	Screenshot(ctx context.Context) (contract.DesktopScreenshot, error)
	Click(ctx context.Context, mark int, expectation string) error
	Type(ctx context.Context, text string, expectation string) error
	Press(ctx context.Context, keys string, expectation string) error
	Drag(ctx context.Context, fromMark int, toMark int, expectation string) error
	Clipboard(ctx context.Context) (string, error)
	SetClipboard(ctx context.Context, text string) error
}

// The desktop keeps the shape the contract is to take. This line fails to build
// the moment one of the eight drifts from it.
var _ desktopAsTheContractShouldRead = (*Desktop)(nil)

// withNoExpectation is the desktop seen through contract.Desktop as it reads
// today, which carries no expectation on any action. It is here so that the
// contract check in internal/testkit still runs against the real desktop while
// the contract waits for its five lines, and it goes when they land.
type withNoExpectation struct {
	desktop *Desktop
}

// Launch opens an application with nothing said about what should happen.
func (seen withNoExpectation) Launch(ctx context.Context, application string) error {
	return seen.desktop.Launch(ctx, application, "")
}

// Screenshot returns the granted window with its controls numbered.
func (seen withNoExpectation) Screenshot(ctx context.Context) (contract.DesktopScreenshot, error) {
	return seen.desktop.Screenshot(ctx)
}

// Click clicks the control with that number.
func (seen withNoExpectation) Click(ctx context.Context, mark int) error {
	return seen.desktop.Click(ctx, mark, "")
}

// Type types text at human pacing.
func (seen withNoExpectation) Type(ctx context.Context, text string) error {
	return seen.desktop.Type(ctx, text, "")
}

// Press presses a key combination.
func (seen withNoExpectation) Press(ctx context.Context, keys string) error {
	return seen.desktop.Press(ctx, keys, "")
}

// Drag drags from one numbered control to another.
func (seen withNoExpectation) Drag(ctx context.Context, fromMark int, toMark int) error {
	return seen.desktop.Drag(ctx, fromMark, toMark, "")
}

// Clipboard reads what is on the machine's clipboard.
func (seen withNoExpectation) Clipboard(ctx context.Context) (string, error) {
	return seen.desktop.Clipboard(ctx)
}

// SetClipboard puts text on the machine's clipboard.
func (seen withNoExpectation) SetClipboard(ctx context.Context, text string) error {
	return seen.desktop.SetClipboard(ctx, text)
}

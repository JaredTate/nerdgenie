package computer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
	"github.com/JaredTate/coeus/internal/tool/loose"
)

// The names a model writes for the fields of one desktop call. The first of each
// is the one the specification asks for, and the rest are the names a model
// half-remembers or brings over from the browser tools.
var (
	actionNames      = []string{"action", "method", "kind", "do"}
	applicationNames = []string{"application", "app", "program", "name"}
	elementNames     = []string{"element", "mark", "control", "number", "ref", "target"}
	textNames        = []string{"text", "value", "content", "input", "to_type"}
	keyNames         = []string{"keys", "key", "combination", "shortcut"}
	toNames          = []string{"to", "to_element", "destination", "onto"}
)

// readInput reads the model's arguments however it wrote them: an action in
// capitals or with a dash in it, an element written the way a browser page names
// one, and a number in quotes.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent, an action, and an expectation")
	if err != nil {
		return input{}, err
	}
	asked := input{Fields: fields}
	action, _ := fields.Text(actionNames...)
	asked.Action = loose.Action(action)
	asked.Intent, asked.WroteIntent = fields.Text(browserread.IntentNames...)
	asked.Application, _ = fields.Text(applicationNames...)
	asked.Element, _ = fields.Mark(elementNames...)
	asked.Text, _ = fields.Text(textNames...)
	asked.Keys, _ = fields.Text(keyNames...)
	asked.To, _ = fields.Mark(toNames...)
	asked.Expectation, asked.WroteExpectation = fields.Text(browserclick.ExpectationNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	return asked, nil
}

// checkCall holds the rules one call must satisfy before the desktop is touched.
// The expectation is asked for inside the switch, because a screenshot and a
// clipboard read change nothing on the screen and there is nothing to expect of
// them.
func checkCall(asked input) error {
	if err := browserread.NeedIntent(asked.Fields, asked.Intent, asked.WroteIntent); err != nil {
		return err
	}
	switch asked.Action {
	case ActionScreenshot, ActionClipboard:
		return nil
	case ActionLaunch:
		if strings.TrimSpace(asked.Application) == "" {
			return errors.New("this call names no application, so say which program to open")
		}
	case ActionClick:
		if err := checkMark(asked.Element, "the control to click"); err != nil {
			return err
		}
	case ActionDrag:
		if err := checkMark(asked.Element, "the control to drag from"); err != nil {
			return err
		}
		if err := checkMark(asked.To, "the control to drag to"); err != nil {
			return err
		}
	case ActionType, ActionSetClipboard:
		if len([]rune(asked.Text)) > MaxTextRunes {
			return fmt.Errorf("this call would type %d characters and the cap is %d, so do it in pieces",
				len([]rune(asked.Text)), MaxTextRunes)
		}
	case ActionKey:
		if strings.TrimSpace(asked.Keys) == "" {
			return errors.New("this call names no keys, so write the combination to press, such as ctrl+s")
		}
	default:
		return fmt.Errorf("the action %q is not one this tool knows, so use launch, screenshot, click, type, key, drag, clipboard, or set_clipboard",
			asked.Action)
	}
	return needExpectation(asked)
}

// needExpectation holds the rule that every step which changes the screen says
// what it expects of the screen, naming the field when the call says nothing.
func needExpectation(asked input) error {
	if !asked.WroteExpectation {
		return asked.Fields.Missing("expectation", "what you expect the screen to do because of this step,")
	}
	if strings.TrimSpace(asked.Expectation) == "" {
		return errors.New("this call says nothing about what should happen, so write in one line what you expect the screen to do")
	}
	if len([]rune(asked.Expectation)) > browserclick.MaxExpectationRunes {
		return fmt.Errorf("the expectation is %d characters and the cap is %d, so say it in one line",
			len([]rune(asked.Expectation)), browserclick.MaxExpectationRunes)
	}
	return nil
}

// checkMark refuses a control number that no screenshot could have given.
func checkMark(mark int, what string) error {
	if mark < 1 {
		return fmt.Errorf("this call does not number %s, so take a screenshot and use a number from it", what)
	}
	return nil
}

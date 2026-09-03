package computer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// checkCall holds the rules one call must satisfy before the desktop is touched.
func checkCall(asked input) error {
	if err := browserread.CheckIntent(asked.Intent); err != nil {
		return err
	}
	if err := browserclick.CheckExpectation(asked.Expectation); err != nil {
		return err
	}
	switch asked.Action {
	case ActionLaunch:
		if strings.TrimSpace(asked.Application) == "" {
			return errors.New("this call names no application, so say which program to open")
		}
	case ActionClick:
		return checkMark(asked.Element, "the control to click")
	case ActionDrag:
		if err := checkMark(asked.Element, "the control to drag from"); err != nil {
			return err
		}
		return checkMark(asked.To, "the control to drag to")
	case ActionType, ActionSetClipboard:
		if len([]rune(asked.Text)) > MaxTextRunes {
			return fmt.Errorf("this call would type %d characters and the cap is %d, so do it in pieces",
				len([]rune(asked.Text)), MaxTextRunes)
		}
	case ActionKey:
		if strings.TrimSpace(asked.Keys) == "" {
			return errors.New("this call names no keys, so write the combination to press, such as ctrl+s")
		}
	case ActionScreenshot, ActionClipboard:
		return nil
	default:
		return fmt.Errorf("the action %q is not one this tool knows, so use launch, screenshot, click, type, key, drag, clipboard, or set_clipboard",
			asked.Action)
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

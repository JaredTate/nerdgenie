package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on. The rules the record itself keeps are left to the record, so that
// there is one place they live and one message the model reads them from.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an operation in it: %w", err)
		}
	}
	if asked.Operation == OperationWhy && strings.TrimSpace(asked.Why) == "" {
		asked.Why = asked.Text
	}
	switch asked.Operation {
	case OperationWhy, OperationDoneWhen, OperationStopWhen, OperationPlan, "":
		return asked, checkSections(asked)
	case OperationDecision:
		return asked, needsText(asked.Text, "a decision is a choice, so write the choice in one line")
	case OperationFailure:
		return asked, needsText(asked.Text, "a failure is something that went wrong, so write what went wrong in one line")
	case OperationPinResult:
		return asked, checkPin(asked)
	default:
		return input{}, fmt.Errorf("the operation %q is not one this tool knows, so use why, done_when, stop_when, plan, decision, failure, or pin_result", asked.Operation)
	}
}

// needsText refuses an operation whose one piece of text is missing.
func needsText(text string, advice string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New(advice)
	}
	return nil
}

// checkList refuses a list that is empty or longer than one task's worth.
func checkList(held int, what string, advice string) error {
	if held == 0 {
		return fmt.Errorf("this call writes an empty %s, so write %s", what, advice)
	}
	if held > MaxLines {
		return fmt.Errorf("this %s has %d lines and the cap is %d, so make a job of the work instead", what, held, MaxLines)
	}
	return nil
}

// checkPin refuses a pin that names no line or no result.
func checkPin(asked input) error {
	if asked.Line < 1 {
		return errors.New("this call names no done line, so say which line the result proves, counting from one")
	}
	if strings.TrimSpace(asked.Result) == "" {
		return errors.New("this call names no result, so give the label of the result that proves the line, such as r7")
	}
	return nil
}

// checkSections refuses a call that writes no section at all, and checks the
// length of every list it does carry.
func checkSections(asked input) error {
	written := 0
	if strings.TrimSpace(asked.Why) != "" {
		written++
	}
	for _, list := range []struct {
		held   int
		what   string
		advice string
	}{
		{len(asked.DoneWhen), "done list", "one line each saying what must be true"},
		{len(asked.StopWhen), "stop list", "one line each saying what stops the work at once"},
		{len(asked.Plan), "plan", "one line per step, in the order they are done"},
	} {
		if list.held == 0 {
			continue
		}
		written++
		if err := checkList(list.held, list.what, list.advice); err != nil {
			return err
		}
	}
	if written == 0 {
		return errors.New(`this call writes nothing, so give "why", "done_when", "stop_when", or "plan", one or several at once`)
	}
	return nil
}

package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on. The rules the record itself keeps are left to the record, so that
// there is one place they live and one message the model reads them from.
func readInput(raw json.RawMessage) (input, error) {
	held := writtenCall{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &held); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an operation in it: %w", err)
		}
	}
	asked, err := readFields(held)
	if err != nil {
		return input{}, err
	}
	operation, err := operationOf(asked)
	if err != nil {
		return input{}, err
	}
	asked.Operation = operation
	fillIn(&asked)
	return asked, check(asked)
}

// readFields reads every field of the call into the shape this tool acts on,
// one field at a time and each under both the names a model gives it.
func readFields(held writtenCall) (input, error) {
	asked := input{Operation: held.Operation}
	texts := []struct {
		raw   json.RawMessage
		field string
		into  *string
	}{
		{held.Why, "why", &asked.Why},
		{held.Text, "text", &asked.Text},
		{held.Reason, "reason", &asked.Reason},
		{held.Cause, "cause", &asked.Cause},
		{eitherRaw(held.Result, held.ResultOneWord), "result", &asked.Result},
	}
	for _, field := range texts {
		read, err := readText(field.raw, field.field)
		if err != nil {
			return input{}, err
		}
		*field.into = read
	}
	return asked, readTheRest(held, &asked)
}

// readTheRest reads the fields that are not one line of text: the three lists,
// the line number, and the decision and the failure written as objects.
func readTheRest(held writtenCall, asked *input) error {
	lists := []struct {
		raw   json.RawMessage
		field string
		into  *[]string
	}{
		{eitherRaw(held.StopWhen, held.StopWhenOneWord), "stop_when", &asked.StopWhen},
		{held.Plan, "plan", &asked.Plan},
	}
	for _, list := range lists {
		read, err := readLines(list.raw, list.field)
		if err != nil {
			return err
		}
		*list.into = read
	}

	doneWhen, err := readDoneLines(eitherRaw(held.DoneWhen, held.DoneWhenOneWord), "done_when")
	if err != nil {
		return err
	}
	asked.DoneWhen = doneWhen
	if asked.Line, err = readNumber(held.Line, "line"); err != nil {
		return err
	}
	if asked.Decision, err = readPair(held.Decision, "decision"); err != nil {
		return err
	}
	asked.Failure, err = readPair(held.Failure, "failure")
	return err
}

// operationOf says which of the seven this call is: the one it names, put in
// this tool's own spelling, or the one the fields it carries can only mean when
// it names none.
func operationOf(asked input) (string, error) {
	if strings.TrimSpace(asked.Operation) == "" {
		return inferredOperation(asked), nil
	}
	known, itIs := theOperations[plainName(asked.Operation)]
	if !itIs {
		return "", fmt.Errorf("the operation %q is not one this tool knows, so use why, done_when, stop_when, plan, decision, failure, or pin_result", asked.Operation)
	}
	return known, nil
}

// theOperations are the seven this tool knows, under the plain name each is
// found by however the model spelled it.
var theOperations = map[string]string{
	"why":       OperationWhy,
	"donewhen":  OperationDoneWhen,
	"stopwhen":  OperationStopWhen,
	"plan":      OperationPlan,
	"decision":  OperationDecision,
	"failure":   OperationFailure,
	"pinresult": OperationPinResult,
}

// theWordsInFront are the words a model puts in front of an operation because
// it is naming what the call does rather than which operation it is.
var theWordsInFront = []string{"set", "add", "update", "write"}

// plainName is the name an operation is found by: the capital letters lowered,
// the underscores, dashes and spaces taken out, and a leading set, add, update
// or write dropped, so that "DONE_WHEN", "doneWhen", "done-when" and
// "set_done_when" are all the one operation and none of them costs a round.
func plainName(operation string) string {
	letters := strings.Map(func(letter rune) rune {
		switch letter {
		case '_', '-', ' ':
			return -1
		}
		return unicode.ToLower(letter)
	}, strings.TrimSpace(operation))
	for _, word := range theWordsInFront {
		if rest := strings.TrimPrefix(letters, word); rest != letters && rest != "" {
			return rest
		}
	}
	return letters
}

// inferredOperation is what a call with no operation in it can only mean, read
// from the fields it carries. The sections come first, because a call carrying
// any of them writes all of them at once, whatever else it holds.
func inferredOperation(asked input) string {
	switch {
	case asked.Why != "" || len(asked.DoneWhen) > 0 || len(asked.StopWhen) > 0 || len(asked.Plan) > 0:
		return operationSections
	case asked.Decision != nil || (asked.Text != "" && asked.Reason != ""):
		return OperationDecision
	case asked.Failure != nil || (asked.Text != "" && asked.Cause != ""):
		return OperationFailure
	case asked.Line > 0 && strings.TrimSpace(asked.Result) != "":
		return OperationPinResult
	default:
		return operationSections
	}
}

// fillIn takes the shapes that mean the same thing and makes them one: a why
// written under "text", a decision or a failure written as two loose fields
// rather than as an object, and the reason or the cause written beside the
// object rather than inside it.
func fillIn(asked *input) {
	if asked.Operation == OperationWhy && strings.TrimSpace(asked.Why) == "" {
		asked.Why = asked.Text
	}
	if asked.Operation == OperationDecision && asked.Decision == nil {
		asked.Decision = &writtenPair{Text: asked.Text}
	}
	if asked.Operation == OperationFailure && asked.Failure == nil {
		asked.Failure = &writtenPair{Text: asked.Text}
	}
	if asked.Decision != nil {
		asked.Decision.Reason = eitherName(asked.Decision.Reason, asked.Reason)
	}
	if asked.Failure != nil {
		asked.Failure.Cause = eitherName(asked.Failure.Cause, asked.Cause)
	}
}

// check holds the rules one call must satisfy before the record ever sees it.
func check(asked input) error {
	switch asked.Operation {
	case OperationDecision:
		return needsText(asked.Decision.Text, "a decision is a choice, so write the choice in one line")
	case OperationFailure:
		return needsText(asked.Failure.Text, "a failure is something that went wrong, so write what went wrong in one line")
	case OperationPinResult:
		return checkPin(asked)
	default:
		return checkSections(asked)
	}
}

// needsText refuses an operation whose one piece of text is missing.
func needsText(text string, advice string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New(advice)
	}
	return nil
}

// checkList refuses a list longer than one task's worth. A list with nothing in
// it is not refused here, because a call that carries no line at all writes no
// section, and the refusal for that names every section the model could write
// instead.
func checkList(held int, what string) error {
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
	sections := 0
	if strings.TrimSpace(asked.Why) != "" {
		sections++
	}
	if asked.Decision != nil || asked.Failure != nil {
		sections++
	}
	for _, list := range []struct {
		held int
		what string
	}{
		{len(asked.DoneWhen), "done list"},
		{len(asked.StopWhen), "stop list"},
		{len(asked.Plan), "plan"},
	} {
		if list.held == 0 {
			continue
		}
		sections++
		if err := checkList(list.held, list.what); err != nil {
			return err
		}
	}
	if sections == 0 {
		return errors.New(`this call writes nothing, so give "why", "done_when", "stop_when", or "plan", one or several at once`)
	}
	return nil
}

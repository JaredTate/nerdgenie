package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// Checker walks an app step by step, photographs every step, and says whether
// each expected state is what the page is showing.
//
// It is a replay that never gives up at the first disappointment, because the
// point of a check is the whole list. It is also a replay that never changes
// anything it was told it could not take back: a check that filed a report would
// not be a check.
type Checker struct {
	worker contract.BrowserWorker
}

// NewChecker returns a checker over one browser worker.
func NewChecker(worker contract.BrowserWorker) (*Checker, error) {
	if worker == nil {
		return nil, errors.New("the checker was built with no browser behind it, so pass the browser worker to walk with")
	}
	return &Checker{worker: worker}, nil
}

// Request is one visual check: what to call it, where the app is, the walk to
// take, and where the pictures go.
type Request struct {
	// Name is what the report is called, and is the address when it is empty.
	Name string
	// Address is where the walk starts. When it is set it replaces the address
	// of the walk's first opening step, so one written walk checks any copy of
	// an app.
	Address string
	// Steps are the walk, each carrying the state expected after it in plain
	// words.
	Steps []Step
	// Into is the folder the labeled pictures are written into.
	Into string
	// CannotBeUndone are the numbers of the steps a check must not take.
	CannotBeUndone []int
}

// Finding is what one step of a check saw.
type Finding struct {
	// Number is the step's place in the walk.
	Number int
	// Label is what the step was for, which is also what its picture is named
	// after.
	Label string
	// State is what was expected after the step, in plain words.
	State string
	// Met says the page bore out the expected state.
	Met bool
	// Seen says what the page was showing, whether or not the state was met.
	Seen string
	// Picture is the path of the labeled screenshot, and is empty when none
	// could be taken.
	Picture string
}

// Result is the whole of one visual check.
type Result struct {
	// Name is what the check was called.
	Name string
	// Address is where the walk started.
	Address string
	// Findings are the steps that were walked, in order.
	Findings []Finding
	// StoppedBecause says why the walk ended early, and is empty when it ran to
	// the end.
	StoppedBecause string
}

// Met says every expected state was borne out and the walk ran to the end.
func (result Result) Met() bool {
	for _, finding := range result.Findings {
		if !finding.Met {
			return false
		}
	}
	return result.StoppedBecause == "" && len(result.Findings) > 0
}

// Markdown is the report: a heading, one line for each step saying whether its
// state was met and what was seen, and the path of the picture taken there.
func (result Result) Markdown() string {
	heading := result.Name
	if heading == "" {
		heading = "visual check of " + result.Address
	}
	lines := []string{"# " + heading, "", "Walked from " + result.Address + ".", ""}
	for _, finding := range result.Findings {
		lines = append(lines, finding.line())
	}
	if result.StoppedBecause != "" {
		lines = append(lines, "- the walk stopped here, because "+result.StoppedBecause)
	}
	return strings.Join(lines, "\n") + "\n"
}

// line is one step's line of the report.
func (finding Finding) line() string {
	verdict := "not met"
	if finding.Met {
		verdict = "met"
	}
	said := fmt.Sprintf("- step %d %s: %s. Expected: %s. Seen: %s.", finding.Number, finding.Label, verdict, finding.State, finding.Seen)
	if finding.Picture != "" {
		said += " Picture: " + finding.Picture
	}
	return said
}

// CheckFolder walks the app a skill folder describes, starting at the address it
// is given rather than the one written into the folder, so that one written walk
// checks any copy of an app.
func (checker *Checker) CheckFolder(ctx context.Context, folder skill.Folder, address string, into string) (Result, error) {
	steps, err := StepsOf(folder)
	if err != nil {
		return Result{}, err
	}
	return checker.Check(ctx, Request{
		Name:           "visual check of the skill " + folder.Definition.Name,
		Address:        address,
		Steps:          steps,
		Into:           into,
		CannotBeUndone: folder.Definition.Permissions.IrreversibleSteps,
	})
}

// Check walks the app and reports one finding per step. A state that is not met
// does not stop the walk, because the point of a check is the whole list; an
// element that cannot be found does, because there is no way to carry on from
// there.
func (checker *Checker) Check(ctx context.Context, request Request) (Result, error) {
	steps, err := checker.prepare(&request)
	if err != nil {
		return Result{}, err
	}

	result := Result{Name: request.Name, Address: request.Address}
	for _, step := range steps {
		if holdsNumber(request.CannotBeUndone, step.Number) {
			result.StoppedBecause = fmt.Sprintf("step %d cannot be undone, and a visual check never changes the app: %s", step.Number, step.Intent)
			break
		}
		page, stopped := checker.walkOneStep(ctx, step)
		result.Findings = append(result.Findings, checker.look(ctx, step, page, request.Into))
		if stopped != "" {
			result.StoppedBecause = stopped
			break
		}
	}
	return result, nil
}

// prepare checks the request, makes the folder for the pictures, and returns the
// walk numbered by position with the address the caller asked for written into
// its first opening step. The numbers are put on here rather than taken as they
// come, because the numbers of the steps a check must not take are matched
// against them and the pictures are named after them: a caller who built the
// request in code and left every number at nought would otherwise get a guard
// that matches nothing and two pictures under one name.
func (checker *Checker) prepare(request *Request) ([]Step, error) {
	if len(request.Steps) == 0 {
		return nil, errors.New("this check has no steps to walk, so write the walk before running it")
	}
	if len(request.Steps) > MaxScreenshots {
		return nil, fmt.Errorf("this check walks %d steps and it takes a picture at each one, and the most it may take is %d, so shorten the walk",
			len(request.Steps), MaxScreenshots)
	}
	if strings.TrimSpace(request.Into) == "" {
		return nil, errors.New("this check has nowhere to put its pictures, so name the folder to write them into")
	}
	if err := os.MkdirAll(request.Into, contract.HomeFolderMode); err != nil {
		return nil, fmt.Errorf("cannot make the folder %s for the pictures, so check that it can be written: %w", request.Into, err)
	}

	steps := make([]Step, len(request.Steps))
	copy(steps, request.Steps)
	for at := range steps {
		steps[at].Number = at + 1
	}
	for at, step := range steps {
		if step.Tool == contract.ToolBrowserOpen && request.Address != "" {
			steps[at].Address = request.Address
			break
		}
	}
	if request.Address == "" {
		request.Address = firstAddress(steps)
	}
	return steps, nil
}

// walkOneStep takes one step of the walk and returns the page it ended on, with
// the reason the walk cannot go on when there is one.
func (checker *Checker) walkOneStep(ctx context.Context, step Step) (contract.Snapshot, string) {
	if step.Tool == contract.ToolBrowserOpen {
		page, err := checker.worker.Open(ctx, step.Address)
		if err != nil {
			return page, fmt.Sprintf("step %d could not open %s: %v", step.Number, step.Address, err)
		}
		return page, ""
	}

	page, err := checker.worker.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return page, fmt.Sprintf("step %d could not read the page it was to act on: %v", step.Number, err)
	}
	ref, _, found := FindElement(page, step.Element)
	if !found {
		return page, fmt.Sprintf("step %d found no element on the page matching %s", step.Number, step.Element)
	}
	change, err := checker.act(ctx, step, ref)
	if err != nil {
		return page, fmt.Sprintf("step %d could not act on %s: %v", step.Number, ref, err)
	}
	return change.Snapshot, ""
}

// act is the one place a step of a walk becomes a call on the browser worker.
func (checker *Checker) act(ctx context.Context, step Step, ref string) (contract.Diff, error) {
	if step.Tool == contract.ToolBrowserType {
		return checker.worker.Type(ctx, ref, step.Typed, step.Expectation)
	}
	return checker.worker.Click(ctx, ref, step.Expectation)
}

// look photographs the page and judges the expected state against it by the word
// rule, which asks what the page is showing rather than what the last action
// changed.
func (checker *Checker) look(ctx context.Context, step Step, page contract.Snapshot, into string) Finding {
	finding := Finding{
		Number: step.Number,
		Label:  step.Intent,
		State:  step.Expectation,
		Met:    StateMet(step.Expectation, page),
		Seen:   WhatIsShown(page),
	}
	picture, err := checker.photograph(ctx, step, into)
	if err != nil {
		finding.Seen = shorten(finding.Seen+" No picture could be taken: "+err.Error(), MaxSeenRunes)
		return finding
	}
	finding.Picture = picture
	return finding
}

// photograph writes the page as a PNG named after the step it shows, so that a
// person reading the report can put a picture beside every line of it.
func (checker *Checker) photograph(ctx context.Context, step Step, into string) (string, error) {
	shot, err := checker.worker.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("the browser could not photograph the page: %w", err)
	}
	picture, err := base64.StdEncoding.DecodeString(shot.PNGBase64)
	if err != nil {
		return "", fmt.Errorf("the picture the browser sent back could not be read: %w", err)
	}
	path := filepath.Join(into, fmt.Sprintf("%02d-%s.png", step.Number, slug(step.Intent)))
	if err := os.WriteFile(path, picture, contract.DataFileMode); err != nil {
		return "", fmt.Errorf("the picture could not be written to %s: %w", path, err)
	}
	return path, nil
}

// maxSlugRunes is how long the part of a picture's name that comes from the step
// may be, because a step's own words are as long as somebody made them.
const maxSlugRunes = 40

// slug turns what a step was for into a file name a person can recognise:
// lowercase words joined by hyphens and nothing else.
func slug(intent string) string {
	words := strings.FieldsFunc(strings.ToLower(intent), notALetterOrDigit)
	joined := strings.Join(words, "-")
	if runes := []rune(joined); len(runes) > maxSlugRunes {
		joined = strings.Trim(string(runes[:maxSlugRunes]), "-")
	}
	if joined == "" {
		return "step"
	}
	return joined
}

package workorder

import (
	"regexp"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/markdown"
)

// The kinds of check the harness can run on a done line, as written in the
// brackets at the line's end.
const (
	// CheckTestsPass runs the command and reads the test result.
	CheckTestsPass = "tests pass"
	// CheckExitZero runs the command and reads its exit code.
	CheckExitZero = "exit 0"
	// CheckShows opens the page in the browser and looks for the quoted text.
	CheckShows = "shows"
	// CheckExists looks for the file.
	CheckExists = "exists"
)

// TestsFirst is the rule the harness puts first in every work order, whether
// or not the person wrote one like it.
const TestsFirst = "Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before the task ends."

// The six headings, matched without regard to case.
const (
	headingGoal     = "goal"
	headingWhere    = "where"
	headingDoneWhen = "done when"
	headingRules    = "rules"
	headingTasks    = "tasks"
	headingDetails  = "details"
)

// Check is what a done line asks the harness to run: the kind, and the rest
// of the bracket after the colon, such as the command, the path, or the quoted
// text and the address.
type Check struct {
	// Kind is one of the four check kinds.
	Kind string
	// Argument is the text after the colon, with its ends trimmed.
	Argument string
}

// DoneLine is one thing that must be true at the end, as the person wrote it.
type DoneLine struct {
	// Text is the line as written, with its bracket kept and its number taken off.
	Text string
	// Check is the check the harness can run, or empty when the line has none.
	Check Check
	// UnknownCheck is the kind of a bracket the harness does not know, so that
	// the line can say so; the bracket stays in the text.
	UnknownCheck string
}

// Task is one line of the order of work.
type Task struct {
	// Text is the task's words without the details suffix.
	Text string
	// Line is the task as written, joined onto one line, details suffix and all.
	Line string
	// Details are the headings of the Details sections the task names.
	Details []string
}

// WorkOrder is an ask read into its parts.
type WorkOrder struct {
	// IsWorkOrder says the text carried a Goal and a Done when heading.
	IsWorkOrder bool
	// Name is the title of the ask, or the first sentence of the goal when
	// there is no title.
	Name string
	// Goal is the paragraph under Goal.
	Goal string
	// Where is the text under Where.
	Where string
	// DoneWhen is the done list in order.
	DoneWhen []DoneLine
	// Rules is the rules in order, as written, without the tests-first line
	// the harness adds.
	Rules []string
	// Tasks is the order of work.
	Tasks []Task
	// Sections is the Details, one section per heading under it.
	Sections []markdown.Section
}

// Parse reads the text. A text without a Goal and a Done when heading comes
// back with IsWorkOrder false and nothing else filled in.
func Parse(text string) WorkOrder {
	order := WorkOrder{}
	var title string
	inDetails := false
	// The section cutter stops a section at the next heading of any level, so
	// the Details heading's own body is empty and each part under it is a
	// deeper section of its own; those are gathered until the next heading
	// at the level of the six.
	for _, section := range markdown.Sections(text) {
		if section.Level == 1 && title == "" {
			title = section.Heading
			continue
		}
		if section.Level > 2 {
			if inDetails {
				order.Sections = append(order.Sections, section)
			}
			continue
		}
		body := strings.TrimSpace(section.Body)
		inDetails = false
		switch strings.ToLower(strings.TrimSpace(section.Heading)) {
		case headingGoal:
			order.Goal = body
		case headingWhere:
			order.Where = body
		case headingDoneWhen:
			order.DoneWhen = doneLinesOf(items(body))
			order.IsWorkOrder = order.Goal != ""
		case headingRules:
			order.Rules = items(body)
		case headingTasks:
			order.Tasks = tasksOf(items(body))
		case headingDetails:
			inDetails = true
		}
	}
	if !order.IsWorkOrder {
		return WorkOrder{}
	}
	order.Name = title
	if order.Name == "" {
		order.Name = firstSentence(order.Goal)
	}
	return order
}

// itemStart opens a list item: a number and a full stop, or a dash or star,
// then a space.
var itemStart = regexp.MustCompile(`^\s*(?:\d+\.|[-*])\s+`)

// items reads a list: every line that opens an item starts one, and every
// other non-empty line continues the item before it, joined with one space.
// Prose before the first item is an introduction, not an item, and is left
// out; a body with no item marks at all is read one line per item.
func items(body string) []string {
	var found []string
	lines := strings.Split(body, "\n")
	listed := false
	for _, line := range lines {
		listed = listed || itemStart.MatchString(line)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if itemStart.MatchString(line) {
			found = append(found, strings.TrimSpace(itemStart.ReplaceAllString(line, "")))
			continue
		}
		if len(found) == 0 {
			if !listed {
				found = append(found, trimmed)
			}
			continue
		}
		found[len(found)-1] += " " + trimmed
	}
	return found
}

// checkAtTheEnd is the bracket that closes a done line: a kind, a colon, and
// the argument.
var checkAtTheEnd = regexp.MustCompile(`\[([^\[\]:]+):\s*([^\]]*)\]\s*$`)

// doneLinesOf reads each item's check off its end.
func doneLinesOf(lines []string) []DoneLine {
	var done []DoneLine
	for _, text := range lines {
		line := DoneLine{Text: text}
		if match := checkAtTheEnd.FindStringSubmatch(text); match != nil {
			kind := strings.ToLower(strings.TrimSpace(match[1]))
			switch kind {
			case CheckTestsPass, CheckExitZero, CheckShows, CheckExists:
				line.Check = Check{Kind: kind, Argument: strings.TrimSpace(match[2])}
			default:
				line.UnknownCheck = kind
			}
		}
		done = append(done, line)
	}
	return done
}

// detailsSuffix is the "(Details: A, B)" a task line may end with.
var detailsSuffix = regexp.MustCompile(`(?i)\s*\(details:\s*([^)]*)\)\s*$`)

// tasksOf reads each item's details off its end.
func tasksOf(lines []string) []Task {
	var tasks []Task
	for _, line := range lines {
		tasks = append(tasks, Task{
			Text:    strings.TrimSpace(detailsSuffix.ReplaceAllString(line, "")),
			Line:    line,
			Details: DetailsNamedBy(line),
		})
	}
	return tasks
}

// DetailsNamedBy reads the headings a task line names in its "(Details: A, B)"
// suffix, or none.
func DetailsNamedBy(line string) []string {
	match := detailsSuffix.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	var names []string
	for _, name := range strings.Split(match[1], ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Headings lists the Details sections' headings in order.
func (order WorkOrder) Headings() []string {
	var headings []string
	for _, section := range order.Sections {
		headings = append(headings, section.Heading)
	}
	return headings
}

// RulesWithTestsFirst is the rules with tests first as the first of them: the
// person's own tests-first line when they wrote one, the harness's when not.
func (order WorkOrder) RulesWithTestsFirst() []string {
	rules := []string{TestsFirst}
	for _, rule := range order.Rules {
		if strings.HasPrefix(strings.ToLower(rule), "tests first") {
			rules[0] = rule
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

// firstSentence is the goal up to its first full stop, for a name when the
// ask has no title.
func firstSentence(goal string) string {
	if at := strings.Index(goal, ". "); at > 0 {
		return goal[:at+1]
	}
	return strings.TrimSpace(goal)
}

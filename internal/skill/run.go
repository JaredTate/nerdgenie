package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

// The bounds on what one replay may produce, so that a skill whose tools return
// pages of text cannot fill the model's context or a message to the user.
const (
	// MaxReportBytes is how long a replay's whole report may be.
	MaxReportBytes = 32 * 1024
	// MaxResultRunes is how much of one step's result the report quotes.
	MaxResultRunes = 200
	// ArgumentsToken is what a step's input writes where the run's own
	// arguments belong.
	ArgumentsToken = "{{arguments}}"
)

// Run replays a skill through the tool registry with no model call at all. The
// permissions block becomes standing approvals first, and every step is put
// through the permission function before it runs. A step that fails hands back
// what it expected and what it saw, along with the report of the steps that did
// work, so that the loop can decide whether the model is needed.
func (store *Store) Run(ctx context.Context, name string, arguments string) (string, error) {
	return store.runOne(ctx, name, arguments, replayKind{})
}

// RunForPerson replays a skill the person at the screen asked for by name,
// which is what "/skills run" calls. It is the same replay, and a yes the person
// gives one of its steps is what a skill the model wrote waits for before its
// permissions block becomes standing approvals. A run the model started never
// counts, because a model that could approve its own skill would be approving
// itself.
func (store *Store) RunForPerson(ctx context.Context, name string, arguments string) (string, error) {
	return store.runOne(ctx, name, arguments, replayKind{person: true})
}

// runOne is the body of both ways of running a skill.
func (store *Store) runOne(ctx context.Context, name string, arguments string, how replayKind) (string, error) {
	folder, err := store.read(name)
	if err != nil {
		return "", err
	}
	if err := store.registerApprovals(folder); err != nil {
		return "", err
	}
	return store.replay(ctx, folder, arguments, how)
}

// DryRun replays a skill up to the first step that cannot be undone and stops,
// with the arguments test.md names. It is what a newly learned skill has to
// pass before it is saved.
func (store *Store) DryRun(ctx context.Context, name string) (string, error) {
	folder, err := store.read(name)
	if err != nil {
		return "", err
	}
	if err := store.registerApprovals(folder); err != nil {
		return "", err
	}
	return store.dryRun(ctx, folder)
}

// replayKind says how a replay was started: whether it is a dry run, which
// stops before the first step that cannot be undone, and whether the person at
// the screen started it, whose yes to a step is what lets a skill the model
// wrote hold a standing approval.
type replayKind struct {
	dry    bool
	person bool
}

// dryRun replays a folder that may not be on disk yet, which is what the
// learning paths use before they save anything.
func (store *Store) dryRun(ctx context.Context, folder Folder) (string, error) {
	report, err := store.replay(ctx, folder, folder.Plan.Arguments, replayKind{dry: true})
	if err != nil {
		return report, err
	}
	if folder.Plan.Expect != "" && !strings.Contains(report, folder.Plan.Expect) {
		return report, fmt.Errorf("the dry run of the skill %q expected to see %q in its report and did not, so fix the steps or the expectation in %s",
			folder.Definition.Name, folder.Plan.Expect, TestFile)
	}
	return report, nil
}

// replay runs the steps in order, checking each one's expectation. A dry run
// stops before the first step the permissions block says cannot be undone.
func (store *Store) replay(ctx context.Context, folder Folder, arguments string, how replayKind) (string, error) {
	steps, err := stepsToReplay(folder)
	if err != nil {
		return "", err
	}

	report := &strings.Builder{}
	for _, step := range steps {
		if how.dry && folder.isIrreversible(step.Number) {
			addLine(report, fmt.Sprintf("step %d stops the dry run, because it cannot be undone: %s", step.Number, step.Intent))
			return report.String(), nil
		}
		if step.Tool == "" {
			return report.String(), fmt.Errorf("step %d of the skill %q is written in words and names no tool, so the model has to carry it out: %s",
				step.Number, folder.Definition.Name, step.Intent)
		}
		output, err := store.runOneStep(ctx, folder, step, arguments, how)
		if err != nil {
			return report.String(), err
		}
		if step.Expect != "" && !strings.Contains(output.Text, step.Expect) {
			return report.String(), fmt.Errorf("step %d of the skill %q expected to see %q and saw %q instead, so the skill needs the model or a fix",
				step.Number, folder.Definition.Name, step.Expect, shorten(output.Text))
		}
		addLine(report, fmt.Sprintf("step %d (%s) did what it said: %s", step.Number, step.Tool, shorten(output.Text)))
	}
	return report.String(), nil
}

// stepsToReplay is the list of steps a replay walks, which is the step list for
// an ordinary skill and one shell step for a skill that carries a script.
func stepsToReplay(folder Folder) ([]Step, error) {
	if folder.HasScript {
		if folder.Path == "" {
			return nil, fmt.Errorf("the skill %q carries a script and is not on disk yet, so save it before running it", folder.Definition.Name)
		}
		input, err := json.Marshal(map[string]string{"command": filepath.Join(folder.Path, ScriptFile) + " " + ArgumentsToken})
		if err != nil {
			return nil, fmt.Errorf("cannot build the command that runs the script of the skill %q, so check the folder name: %w", folder.Definition.Name, err)
		}
		return []Step{{Number: 1, Intent: "Run the script of this skill.", Tool: contract.ToolShell, Input: string(input)}}, nil
	}
	if len(folder.Steps) == 0 {
		return nil, fmt.Errorf("the skill %q has no steps to run, so write its procedure into %s", folder.Definition.Name, StepsFile)
	}
	return folder.Steps, nil
}

// runOneStep rules on one step and then runs it.
func (store *Store) runOneStep(ctx context.Context, folder Folder, step Step, arguments string, how replayKind) (contract.ToolOutput, error) {
	input := json.RawMessage(substituteArguments(step.Input, arguments))
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	if err := store.allowStep(ctx, folder, step, input, how); err != nil {
		return contract.ToolOutput{}, err
	}
	tool, found := store.tools.Lookup(step.Tool)
	if !found {
		return contract.ToolOutput{}, fmt.Errorf("step %d of the skill %q calls the tool %q, which this machine does not have, so change the step to one it does have",
			step.Number, folder.Definition.Name, step.Tool)
	}
	output, err := tool.Run(ctx, input)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("step %d of the skill %q failed while running %s, so the skill needs the model or a fix: %w",
			step.Number, folder.Definition.Name, step.Tool, err)
	}
	return output, nil
}

// allowStep puts one step through the permissions block, the permission
// function, and the preview a step that cannot be undone needs, in that order.
func (store *Store) allowStep(ctx context.Context, folder Folder, step Step, input json.RawMessage, how replayKind) error {
	if site, outside := siteOutsideTheBlock(folder.Definition.Permissions, step, input); outside {
		why := fmt.Sprintf("it visits %s, which the permissions block of this skill does not name", site)
		return store.askTheUser(ctx, folder, step, why, string(input), how)
	}

	decision, err := store.permission.Decide(ctx, contract.PermissionRequest{ToolName: step.Tool, Input: input})
	if err != nil {
		return fmt.Errorf("cannot rule on step %d of the skill %q, so check the permission rules: %w", step.Number, folder.Definition.Name, err)
	}
	switch decision.Ruling {
	case contract.RulingAllow:
	case contract.RulingDeny:
		return fmt.Errorf("step %d of the skill %q is refused by your rules, so change the rule or the step: %s",
			step.Number, folder.Definition.Name, decision.Reason)
	default:
		if err := store.askTheUser(ctx, folder, step, decision.Reason, decision.PreviewText, how); err != nil {
			return err
		}
	}
	if folder.isIrreversible(step.Number) {
		return store.askTheUser(ctx, folder, step, "it cannot be undone", string(input), how)
	}
	return nil
}

// askTheUser shows one step and waits for a yes. With no channel wired it stops
// and says what it needed, which is what an unattended run gets.
func (store *Store) askTheUser(ctx context.Context, folder Folder, step Step, why string, body string, how replayKind) error {
	name := folder.Definition.Name
	if store.ask == nil {
		return fmt.Errorf("step %d of the skill %q needs your yes, because %s, and there is no screen to ask on, so run the skill yourself",
			step.Number, name, why)
	}
	answer, err := store.ask(ctx, contract.Preview{
		ID:    fmt.Sprintf("%s-step-%d", name, step.Number),
		Title: fmt.Sprintf("Step %d of the skill %q needs your yes, because %s.", step.Number, name, why),
		Body:  body,
	})
	if err != nil {
		return fmt.Errorf("cannot ask you about step %d of the skill %q, so check the screen you are on: %w", step.Number, name, err)
	}
	if answer.Answer == contract.AnswerReject {
		if reason := strings.TrimSpace(answer.Reason); reason != "" {
			return fmt.Errorf("step %d of the skill %q was refused, so the skill stopped there, and you said: %s", step.Number, name, reason)
		}
		return fmt.Errorf("step %d of the skill %q was refused, so the skill stopped there and did nothing more", step.Number, name)
	}
	if how.person {
		store.rememberThePersonSaidYes(name)
	}
	return nil
}

// siteOutsideTheBlock says whether a step goes to a website the permissions
// block does not name. Only a step that carries an address is checked, because
// a step with no address visits nothing.
func siteOutsideTheBlock(permissions Permissions, step Step, input json.RawMessage) (string, bool) {
	if !visitsAWebsite(step.Tool) {
		return "", false
	}

	address := addressIn(input)
	if address == "" {
		return "", false
	}
	host := hostOf(address)
	for _, site := range permissions.Sites {
		if host == site || strings.HasSuffix(host, "."+site) {
			return "", false
		}
	}
	return address, true
}

// addressIn returns the web address a step's input carries, reading the fields
// the browser and web tools take.
func addressIn(input json.RawMessage) string {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	for _, name := range []string{"url", "site"} {
		written, held := fields[name]
		if !held {
			continue
		}
		address := ""
		if err := json.Unmarshal(written, &address); err == nil && strings.TrimSpace(address) != "" {
			return strings.TrimSpace(address)
		}
	}
	return ""
}

// hostOf returns the host of a web address, folded to lowercase and without any
// port, and returns the address itself when it is a bare host name.
func hostOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return strings.ToLower(strings.TrimSpace(address))
	}
	return strings.ToLower(parsed.Hostname())
}

// registerApprovals turns a skill's permissions block into standing approvals
// in the permission function: one for each website the block names, covering the
// tools that visit a website, good for the daily limit and running out at
// midnight. They are registered once a day per skill, so that running a skill
// twice does not hand it twice its budget, and a skill the model wrote gets none
// of them until a person has run it and said yes.
//
// The site is checked here as well as where SKILL.md was read, because this is
// the place a site line turns into permission to act, and a folder can reach it
// from anywhere a Definition is built.
func (store *Store) registerApprovals(folder Folder) error {
	permissions := folder.Definition.Permissions
	if store.standing == nil || len(permissions.Sites) == 0 {
		return nil
	}
	if store.waitingForAPersonToRunIt(folder) {
		return nil
	}
	now := store.clock.Now()
	today := now.Format(time.DateOnly)

	store.guard.Lock()
	defer store.guard.Unlock()
	if store.registered[folder.Definition.Name] == today {
		return nil
	}
	for _, site := range permissions.Sites {
		if err := store.registerOneSite(folder, site, now); err != nil {
			return err
		}
	}
	store.registered[folder.Definition.Name] = today
	return nil
}

// registerOneSite gives the permission function the standing approval for one
// website: the host as the person typed it, and the tools whose call names the
// website it goes to, so that the permission function decides by comparing hosts
// and an approval for a website can never cover a command run on this machine.
func (store *Store) registerOneSite(folder Folder, site string, now time.Time) error {
	name := folder.Definition.Name
	if err := checkSite(site); err != nil {
		return fmt.Errorf("the skill %q names the website %q in its permissions block, and a standing approval is built from one bare host name, because %w", name, site, err)
	}
	approval := permission.StandingApproval{
		Skill:   name,
		Host:    site,
		Tools:   permission.ToolsThatVisitAWebsite(),
		Limit:   folder.Definition.Permissions.DailyLimit,
		Expires: nextMidnight(now),
	}
	if err := store.standing.RegisterStandingApproval(approval); err != nil {
		return fmt.Errorf("cannot give the skill %q its standing approval for %s, so check its permissions block: %w", name, site, err)
	}
	return nil
}

// nextMidnight is the start of the next day where the machine is, which is when
// a skill's daily budget starts again.
func nextMidnight(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
}

// substituteArguments puts the run's own arguments wherever a step's input
// wrote the arguments token, escaped so that the input is still JSON whatever
// the user typed.
func substituteArguments(input string, arguments string) string {
	if !strings.Contains(input, ArgumentsToken) {
		return input
	}
	quoted, err := json.Marshal(arguments)
	if err != nil {
		return strings.ReplaceAll(input, ArgumentsToken, "")
	}
	return strings.ReplaceAll(input, ArgumentsToken, strings.Trim(string(quoted), `"`))
}

// addLine adds one line to a replay's report, and stops adding once the report
// is as long as a report is allowed to be.
func addLine(report *strings.Builder, line string) {
	if report.Len()+len(line) > MaxReportBytes {
		return
	}
	report.WriteString(line)
	report.WriteString("\n")
}

// shorten cuts a tool's result down to what a report quotes of it.
func shorten(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	runes := []rune(text)
	if len(runes) <= MaxResultRunes {
		return text
	}
	return string(runes[:MaxResultRunes]) + " (cut short before the end)"
}

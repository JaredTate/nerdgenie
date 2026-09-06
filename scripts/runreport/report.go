package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// numbers is what one run measured as.
type numbers struct {
	taskID           string
	status           contract.RecordStatus
	rounds           int
	minutes          float64
	callsByTool      map[string]int
	repliesBatched   int
	recordOnlyRounds int
	testOnlyRounds   int
	tokensIn         int
	cachedIn         int
	tokensOut        int
	rewinds          int
	failures         int
	firstLineMarks   int
	roundsAtTheCap   int
	// tasks is how many tasks the numbers cover: one, or a range from an
	// ask's own task on when the ask became a job.
	tasks int
}

// measureFrom reads every task from the one numbered to the newest, together,
// which is the number for an ask that became a job: the sums of the rounds,
// the minutes, the calls and the tokens, the newest task's status, and the
// range as the label.
func measureFrom(ctx context.Context, store contract.Store, from string) (numbers, error) {
	newest, err := newestTask(ctx, store)
	if err != nil {
		return numbers{}, err
	}
	first, last := 0, 0
	if _, err := fmt.Sscanf(from, "%d", &first); err != nil || first < 1 {
		return numbers{}, fmt.Errorf("the task to measure from is %q, and it must be a task number", from)
	}
	if _, err := fmt.Sscanf(newest, "%d", &last); err != nil || last < first {
		return numbers{}, fmt.Errorf("the log holds no task from %d on", first)
	}
	summed := numbers{taskID: from, callsByTool: map[string]int{}}
	for number := first; number <= last; number++ {
		one, err := measure(ctx, store, fmt.Sprintf("%d", number))
		if err != nil {
			continue
		}
		summed.tasks++
		summed.status = one.status
		summed.rounds += one.rounds
		summed.minutes += one.minutes
		summed.repliesBatched += one.repliesBatched
		summed.recordOnlyRounds += one.recordOnlyRounds
		summed.testOnlyRounds += one.testOnlyRounds
		summed.tokensIn += one.tokensIn
		summed.cachedIn += one.cachedIn
		summed.tokensOut += one.tokensOut
		summed.rewinds += one.rewinds
		summed.failures += one.failures
		summed.firstLineMarks += one.firstLineMarks
		summed.roundsAtTheCap += one.roundsAtTheCap
		for name, count := range one.callsByTool {
			summed.callsByTool[name] += count
		}
	}
	if summed.tasks == 0 {
		return numbers{}, fmt.Errorf("the log holds no measurable task from %d on", first)
	}
	if first != last {
		summed.taskID = fmt.Sprintf("%d to %d", first, last)
	}
	return summed, nil
}

// OutputCapReached is the output count, rounded to the hundred the record
// keeps, at which a round's reply reached the default output cap of 8,192
// tokens and was cut off: the tenth nightly run lost six rounds of the game
// job that way, each two minutes of generation kept nowhere.
const OutputCapReached = 8100

// theFirstLineMark finds a step or a line marked done on the model's first
// line, which every checkpoint keeps as the situation's "where the work
// stands" line.
var theFirstLineMark = regexp.MustCompile(`(?i)\b(?:step|line) \d+ done`)

// theTestRunners are the words in a shell command that make it a test run.
var theTestRunners = []string{"--test", "npm test", "pytest", "go test", "test/run", "vitest", "jest"}

// measure reads one task's run out of the store. The task is the one numbered,
// or the newest task in the log when the number is empty.
func measure(ctx context.Context, store contract.Store, taskID string) (numbers, error) {
	if taskID == "" {
		newest, err := newestTask(ctx, store)
		if err != nil {
			return numbers{}, err
		}
		taskID = newest
	}
	events, err := store.ByTask(ctx, taskID)
	if err != nil {
		return numbers{}, fmt.Errorf("cannot read task %s out of the log: %w", taskID, err)
	}
	if len(events) == 0 {
		return numbers{}, fmt.Errorf("the log holds no events for task %s", taskID)
	}
	measured := numbers{taskID: taskID, callsByTool: map[string]int{}}
	var first, last time.Time
	round := []contract.ToolCall{}
	for _, event := range events {
		if first.IsZero() {
			first = event.Occurred
		}
		switch event.Kind {
		case contract.EventCheckpoint:
			measured.countTheRound(round)
			round = nil
			measured.readTheCheckpoint(event)
			last = event.Occurred
		case contract.EventToolCall:
			call := contract.ToolCall{}
			if err := json.Unmarshal(event.Body, &call); err == nil {
				round = append(round, call)
				measured.callsByTool[call.Name]++
			}
		}
	}
	measured.countTheRound(round)
	if !last.IsZero() {
		measured.minutes = last.Sub(first).Minutes()
	}
	return measured, nil
}

// readTheCheckpoint adds one checkpoint's cost to the totals and takes its
// status, rewinds and failures as the run's newest.
func (measured *numbers) readTheCheckpoint(event contract.Event) {
	saved := record.Checkpoint{}
	if err := json.Unmarshal(event.Body, &saved); err != nil {
		return
	}
	measured.rounds++
	held, err := record.Parse([]byte(saved.Text))
	if err != nil {
		return
	}
	measured.status = held.Header.Status
	for _, fact := range held.Work.Situation {
		if orient, found := strings.CutPrefix(fact, "where the work stands: "); found {
			measured.firstLineMarks += len(theFirstLineMark.FindAllString(orient, -1))
		}
	}
	measured.tokensIn += held.Header.Cost.InputTokens
	measured.cachedIn += held.Header.Cost.CachedInputTokens
	measured.tokensOut += held.Header.Cost.OutputTokens
	if held.Header.Cost.OutputTokens >= OutputCapReached {
		measured.roundsAtTheCap++
	}
	measured.failures = len(held.Lessons.Failures)
	measured.rewinds = 0
	for _, failure := range held.Lessons.Failures {
		if strings.HasPrefix(failure.Text, "stalled:") {
			measured.rewinds++
		}
	}
}

// countTheRound reads the calls of one round: several calls in one reply is a
// batch, calls that only wrote the record are a record-only round, and one
// shell call that ran the tests is a test-only round.
func (measured *numbers) countTheRound(calls []contract.ToolCall) {
	if len(calls) == 0 {
		return
	}
	if len(calls) > 1 {
		measured.repliesBatched++
	}
	onlyRecord := true
	for _, call := range calls {
		if call.Name != contract.ToolTask {
			onlyRecord = false
		}
	}
	if onlyRecord {
		measured.recordOnlyRounds++
	}
	if len(calls) == 1 && calls[0].Name == contract.ToolShell && runsTheTests(calls[0]) {
		measured.testOnlyRounds++
	}
}

// runsTheTests says whether a shell call's command is a test run.
func runsTheTests(call contract.ToolCall) bool {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(call.Input, &fields); err != nil {
		return false
	}
	command := ""
	if raw, there := fields["command"]; there {
		_ = json.Unmarshal(raw, &command)
	}
	for _, runner := range theTestRunners {
		if strings.Contains(command, runner) {
			return true
		}
	}
	return false
}

// newestTask is the highest-numbered task the log holds a checkpoint for.
func newestTask(ctx context.Context, store contract.Store) (string, error) {
	saved, err := store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return "", fmt.Errorf("cannot read the checkpoints out of the log: %w", err)
	}
	newest, highest := "", -1
	for _, event := range saved {
		number := 0
		if _, err := fmt.Sscanf(event.TaskID, "%d", &number); err != nil {
			continue
		}
		if number > highest {
			newest, highest = event.TaskID, number
		}
	}
	if newest == "" {
		return "", fmt.Errorf("the log holds no task checkpoints")
	}
	return newest, nil
}

// String writes the numbers as the short table the progress document carries.
func (measured numbers) String() string {
	tools := make([]string, 0, len(measured.callsByTool))
	for name := range measured.callsByTool {
		tools = append(tools, name)
	}
	sort.Strings(tools)
	byTool := make([]string, 0, len(tools))
	calls := 0
	for _, name := range tools {
		byTool = append(byTool, fmt.Sprintf("%s %d", name, measured.callsByTool[name]))
		calls += measured.callsByTool[name]
	}
	cache := 0.0
	if measured.tokensIn > 0 {
		cache = 100 * float64(measured.cachedIn) / float64(measured.tokensIn)
	}
	seconds := 0.0
	if measured.rounds > 0 {
		seconds = measured.minutes * 60 / float64(measured.rounds)
	}
	return fmt.Sprintf("task %s: %s after %d rounds in %.0f minutes (%.0f s a round)\n"+
		"calls: %d (%s); replies with more than one call: %d\n"+
		"rounds that only wrote the record: %d; rounds that only ran the tests: %d\n"+
		"tokens: %.1fk in, %.1fk of them cached (%.0f%%), %.1fk out\n"+
		"rewinds: %d; failures on the record: %d; marks made from the first line: %d; rounds cut off at the output cap: %d",
		measured.taskID, measured.status, measured.rounds, measured.minutes, seconds,
		calls, strings.Join(byTool, ", "), measured.repliesBatched,
		measured.recordOnlyRounds, measured.testOnlyRounds,
		float64(measured.tokensIn)/1000, float64(measured.cachedIn)/1000, cache, float64(measured.tokensOut)/1000,
		measured.rewinds, measured.failures, measured.firstLineMarks, measured.roundsAtTheCap)
}

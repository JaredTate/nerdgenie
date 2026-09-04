package loop

import (
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// This file is everything about a task's budget, which is design section 3,
// rule 3 under the user's own rule: Nerd Genie puts no cap on its own work unless
// the user asks for one. A task's rounds and time are zero unless the caps or
// a skill set them, and a zero means no limit everywhere below.

// budgetRounds is how many rounds this task may take: its own budget when a
// skill set one, and the cap otherwise. Zero, which is what the caps ship
// with, means no limit.
func budgetRounds(task Task, caps contract.Caps) int {
	if task.Budget.Rounds > 0 {
		return task.Budget.Rounds
	}
	return caps.RoundsPerTask
}

// budgetTime is how long this task may take, the same way.
func budgetTime(task Task, caps contract.Caps) time.Duration {
	if task.Budget.Time > 0 {
		return task.Budget.Time
	}
	return caps.TimePerTask
}

// describeBudget says in plain words what a task may spend, for the situation
// line a continued task carries: "no budget", or the rounds, the minutes, or
// both.
func describeBudget(rounds int, allowed time.Duration) string {
	parts := []string{}
	if rounds > 0 {
		parts = append(parts, fmt.Sprintf("%d rounds", rounds))
	}
	if allowed > 0 {
		parts = append(parts, fmt.Sprintf("%d minutes", int(allowed/time.Minute)))
	}
	if len(parts) == 0 {
		return "no budget"
	}
	return strings.Join(parts, " and ")
}

// mayPlayRound says whether this round of the loop may run: always, on a task
// with no round budget, and up to the budget and the spare rounds otherwise.
func (running *run) mayPlayRound(round int) bool {
	return running.roundsAllowed <= 0 || round < running.roundsAllowed+extraRounds
}

// budgetIsSpent says why the task's budget is gone, and is empty while it has
// budget left. A limit the user did not set is off, and a task with both off
// is never stopped here.
func (running *run) budgetIsSpent() string {
	if running.roundsAllowed > 0 && running.roundsUsed >= running.roundsAllowed {
		return fmt.Sprintf("the budget of %d rounds is used up", running.roundsAllowed)
	}
	if running.timeAllowed > 0 && running.spent() >= running.timeAllowed {
		return fmt.Sprintf("the budget of %s is used up", running.timeAllowed)
	}
	return ""
}

// spent is how long this task has been running on the harness's clock.
func (running *run) spent() time.Duration {
	return running.theLoop.options.Clock.Now().Sub(running.startedAt)
}

// budgetLeft is how much of the task's budget is left on each of the limits it
// has, as the record's header carries it. A limit the user did not set is off,
// and the header says so rather than counting down from nothing.
func (running *run) budgetLeft() record.Budget {
	return record.Budget{
		RoundsLeft:    max(running.roundsAllowed-running.roundsUsed, 0),
		NoRoundBudget: running.roundsAllowed <= 0,
		MinutesLeft:   max(int((running.timeAllowed-running.spent())/time.Minute), 0),
		NoTimeBudget:  running.timeAllowed <= 0,
	}
}

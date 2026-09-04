package main

import (
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheBudgetLineSaysNoBudgetUnlessOneIsSet is the status line's half of the
// user's rule: the strip at the bottom of the screen used to say "100 rounds,
// 1h per task" on every install, and there is no such limit unless the user
// set one. The line follows the record's header: "no budget", or only the
// limit that is set.
func TestTheBudgetLineSaysNoBudgetUnlessOneIsSet(t *testing.T) {
	for _, one := range []struct {
		name   string
		rounds int
		task   time.Duration
		want   string
	}{
		{"nothing set, which is the shipped default", 0, 0, "no budget"},
		{"rounds only", 100, 0, "100 rounds per task"},
		{"time only", 0, time.Hour, "1h per task"},
		{"under a minute, which the header spells out", 0, 30 * time.Second, "30s per task"},
		{"both", 40, 30 * time.Minute, "40 rounds, 30m per task"},
	} {
		running := &agent{settings: contract.DefaultConfig()}
		running.settings.Caps.RoundsPerTask = one.rounds
		running.settings.Caps.TimePerTask = one.task
		if got := running.budgetLine(); got != one.want {
			t.Errorf("with %s the budget line reads %q, want %q", one.name, got, one.want)
		}
	}
}

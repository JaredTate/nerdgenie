package main

import (
	"context"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
)

// budgetForTheMessage is the budget the task this message starts runs on: the
// rounds and the time of the skill whose trigger words the message holds, when
// there is one and it sets them, and nothing otherwise, which the loop reads as
// the caps in the configuration. Design section 3, rule 3: every task has a
// budget, and a skill can set its own.
//
// A skill store that cannot answer, or that is not open yet, costs the task its
// skill's budget and nothing more, because a task on the caps is better than no
// task at all.
func (running *agent) budgetForTheMessage(ctx context.Context, message contract.Inbound) loop.Budget {
	if running.skillsBox == nil {
		return loop.Budget{}
	}
	match, err := running.skillsBox.Match(ctx, message.Text)
	if err != nil || !match.Matched {
		return loop.Budget{}
	}
	return loop.Budget{Rounds: match.Rounds, Time: match.Time}
}

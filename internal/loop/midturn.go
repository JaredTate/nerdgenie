// A message that arrives in the middle of a turn steering the next step,
// rather than interrupting the step in flight, is OpenClaw's design, from the
// agent loop at ~/Code/openclaw/packages/agent-core/src/agent-loop.ts and its
// types at ~/Code/openclaw/packages/agent-core/src/types.ts. The Go here is
// written fresh.

package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// theWordsThatMeanStop is the short fixed list of things a user says when they
// want the task to stop. The whole message has to be one of them, so that "do
// not stop until it is posted" is a correction and not a stop.
var theWordsThatMeanStop = []string{"stop", "halt", "cancel", "abort", "quit", "never mind", "nevermind", "forget it"}

// readDelivered takes the messages that arrived while the tools were running.
// The harness copies the user's words into the record first, word for word, so
// that the correction is there whatever the model does with it next.
func (running *run) readDelivered(ctx context.Context) (Outcome, bool, error) {
	for _, message := range running.theLoop.takeDelivered() {
		if meansStop(message.Text) {
			outcome, err := running.stopHere(ctx, "the user said to stop")
			return outcome, false, err
		}
		if err := running.takeTheCorrection(ctx, message); err != nil {
			return Outcome{}, false, err
		}
	}
	return Outcome{}, true, nil
}

// takeTheCorrection writes the user's words into the record and puts them in
// front of the model, and lets the model try again what it had been refused,
// because the world has changed.
func (running *run) takeTheCorrection(ctx context.Context, message contract.Inbound) error {
	running.hadCorrection = true
	running.forgetTheCalls()
	running.remember(contract.Message{Role: contract.RoleUser, Text: message.Text})
	if err := running.theLoop.logEvent(ctx, running.number, contract.EventMessage, message); err != nil {
		return err
	}
	if running.keeper == nil {
		return nil
	}
	if _, err := running.keeper.AddCorrection(ctx, message.Text); err != nil {
		return fmt.Errorf("cannot write what the user said into the record of task %s: %w", running.keeper.ID(), err)
	}
	return nil
}

// meansStop says whether a message is the user telling the task to stop.
func meansStop(said string) bool {
	plain := strings.ToLower(strings.TrimSpace(said))
	plain = strings.Trim(plain, ".!,")
	for _, word := range theWordsThatMeanStop {
		if plain == word {
			return true
		}
	}
	return false
}

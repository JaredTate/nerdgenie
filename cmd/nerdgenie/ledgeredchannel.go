package main

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
)

// ledgeredChannel is a channel whose replies go through the delivery ledger:
// every one is written into the event log before it is sent and marked once it
// has been, so that a crash between the two cannot lose the user's answer and
// the next start can send again what never arrived.
//
// It wraps the channel rather than living inside it because the ledger is the
// reliability guard's and a channel knows nothing about guards; everything else
// the channel does is its own work, untouched.
type ledgeredChannel struct {
	contract.Channel
	guard *reliability.Guard
}

// throughTheLedger wraps one channel so that its replies are written down first.
func throughTheLedger(where contract.Channel, guard *reliability.Guard) contract.Channel {
	if guard == nil {
		return where
	}
	return ledgeredChannel{Channel: where, guard: guard}
}

// Send writes the reply down, sends it, and marks it delivered.
func (ledgered ledgeredChannel) Send(ctx context.Context, text string) error {
	return ledgered.guard.Deliver(ctx, "", ledgered.Channel.Name(), text)
}

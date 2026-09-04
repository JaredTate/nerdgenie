package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxPreviewsWaiting is how many questions may be waiting for an answer at one
// time. One task asks one question at a time, so a handful is already more than
// the agent can be in the middle of.
const maxPreviewsWaiting = 32

// waitingPreviews remembers every preview that is on a screen with nobody's
// answer yet, so that "/status" can list them and "/approve 3" and "/deny 3" can
// answer one by its number.
//
// It exists because the channel that shows a preview keeps its own waiting list
// private: whoever asked is waiting inside ShowPreview, and there is no way in
// from outside. So the wrapper below asks the channel in a goroutine of its own
// and waits on both answers at once, whichever comes first, and gives the other
// one up.
type waitingPreviews struct {
	guard   sync.Mutex
	waiting map[string]waitingPreview
}

// waitingPreview is one question on a screen and the way an answer reaches it.
type waitingPreview struct {
	// preview is what the user is being shown.
	preview contract.Preview
	// answer carries the answer a slash command gave, and holds one.
	answer chan contract.PreviewAnswerWithReason
}

// newWaitingPreviews returns an empty list.
func newWaitingPreviews() *waitingPreviews {
	return &waitingPreviews{waiting: map[string]waitingPreview{}}
}

// begin writes one preview down and gives back the way an answer reaches it.
func (held *waitingPreviews) begin(preview contract.Preview) (chan contract.PreviewAnswerWithReason, error) {
	held.guard.Lock()
	defer held.guard.Unlock()
	if len(held.waiting) >= maxPreviewsWaiting {
		return nil, fmt.Errorf("%d questions are already waiting for an answer, which is as many as this agent holds at once", len(held.waiting))
	}
	answer := make(chan contract.PreviewAnswerWithReason, 1)
	held.waiting[preview.ID] = waitingPreview{preview: preview, answer: answer}
	return answer, nil
}

// end forgets one preview, whichever way it was answered.
func (held *waitingPreviews) end(id string) {
	held.guard.Lock()
	defer held.guard.Unlock()
	delete(held.waiting, id)
}

// list is every preview waiting for an answer, which is what "/status" prints.
func (held *waitingPreviews) list(_ context.Context) ([]contract.Preview, error) {
	held.guard.Lock()
	defer held.guard.Unlock()
	waiting := make([]contract.Preview, 0, len(held.waiting))
	for _, one := range held.waiting {
		waiting = append(waiting, one.preview)
	}
	return waiting, nil
}

// answer gives one waiting preview the answer a slash command typed.
func (held *waitingPreviews) answer(_ context.Context, id string, given contract.PreviewAnswer, reason string) error {
	held.guard.Lock()
	one, there := held.waiting[id]
	held.guard.Unlock()
	if !there {
		return fmt.Errorf("nothing is waiting to be answered under the number %q, so check the number on the question", id)
	}

	select {
	case one.answer <- contract.PreviewAnswerWithReason{Answer: given, Reason: reason}:
		return nil
	default:
		return fmt.Errorf("the question numbered %q has already been answered, so there is nothing left to answer", id)
	}
}

// watchedChannel is a channel whose questions are written down while they wait.
// Everything else it does is the channel's own work, untouched.
type watchedChannel struct {
	contract.Channel
	waiting *waitingPreviews
}

// ShowPreview shows the question on the screen and waits for whichever answer
// comes first: the one a screen sent back, or the one somebody typed as
// "/approve 3" or "/deny 3". The other way of answering is given up as soon as
// one of them lands, so nothing is left waiting behind.
func (watched watchedChannel) ShowPreview(ctx context.Context, preview contract.Preview) (contract.PreviewAnswerWithReason, error) {
	typed, err := watched.waiting.begin(preview)
	if err != nil {
		return contract.PreviewAnswerWithReason{Answer: contract.AnswerReject, Reason: err.Error()}, nil
	}
	defer watched.waiting.end(preview.ID)

	onTheScreen, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	fromTheScreen := make(chan answeredPreview, 1)
	go func() {
		given, err := watched.Channel.ShowPreview(onTheScreen, preview)
		fromTheScreen <- answeredPreview{given: given, err: err}
	}()

	select {
	case given := <-typed:
		return given, nil
	case shown := <-fromTheScreen:
		return shown.given, shown.err
	}
}

// answeredPreview is what the channel itself answered, or why it could not.
type answeredPreview struct {
	// given is the answer the channel brought back.
	given contract.PreviewAnswerWithReason
	// err says the channel could not ask at all.
	err error
}

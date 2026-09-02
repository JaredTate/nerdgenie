package channel

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// ShowPreview puts exactly what is about to happen in front of every attached
// screen and waits for the first answer any of them gives.
//
// It answers no in three cases: no screen is attached, so nobody could have said
// yes; nobody answered before the deadline; or the caller gave up. The first two
// come back with no error, because a no is a real answer and nothing on the
// ask-me-first list may happen without a yes. The third comes back with the
// caller's own reason as the error.
func (socket *Socket) ShowPreview(ctx context.Context, preview contract.Preview) (contract.PreviewAnswer, error) {
	id := preview.ID
	if id == "" {
		id = socket.nextAskID()
	}
	waiting := make(chan contract.PreviewAnswer, 1)
	socket.waitOnPreview(id, waiting)
	defer socket.stopWaitingOnPreview(id)

	shown, err := socket.writeToScreens(ctx, contract.SocketEnvelope{
		Type:  contract.SocketPreview,
		ID:    id,
		Title: preview.Title,
		Text:  preview.Body,
	})
	if err != nil {
		return contract.AnswerReject, err
	}
	if shown == 0 {
		return contract.AnswerReject, nil
	}

	answer, answered := waitForAnswer(ctx, socket.options.Clock, socket.options.AnswerDeadline, waiting)
	if !answered {
		return contract.AnswerReject, contextTrouble(ctx)
	}
	return answer, nil
}

// waitForAnswer waits for one answer from a screen and gives up when the
// deadline passes or the caller's context is cancelled, so that no question ever
// waits for ever. It says whether an answer arrived.
func waitForAnswer[Answer any](ctx context.Context, clock contract.Clock, deadline time.Duration, waiting <-chan Answer) (Answer, bool) {
	var nothing Answer

	timing, stopTiming := context.WithCancel(ctx)
	defer stopTiming()
	late := make(chan struct{})
	go func() {
		defer close(late)
		_ = clock.Sleep(timing, deadline)
	}()

	select {
	case answer := <-waiting:
		return answer, true
	case <-late:
		return nothing, false
	case <-ctx.Done():
		return nothing, false
	}
}

// contextTrouble returns the caller's own reason for giving up, and nothing when
// the caller did not give up.
func contextTrouble(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("nobody answered the preview: %w", err)
	}
	return nil
}

// waitOnPreview writes down that someone is waiting for an answer to a preview.
func (socket *Socket) waitOnPreview(id string, waiting chan contract.PreviewAnswer) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	socket.previews[id] = waiting
}

// stopWaitingOnPreview forgets a preview whose answer has come or whose time has
// run out, so that a late answer is told there is nothing to answer.
func (socket *Socket) stopWaitingOnPreview(id string) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	delete(socket.previews, id)
}

// nextAskID gives a question its own number, which is what a screen answers
// with. The letter keeps these apart from the numbers the permission function
// puts on its own previews.
func (socket *Socket) nextAskID() string {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	socket.asked++
	return "a" + strconv.FormatInt(socket.asked, 10)
}

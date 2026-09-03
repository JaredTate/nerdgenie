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
	if !socket.waitOnPreview(id, waiting) {
		return contract.AnswerReject, fmt.Errorf("a preview numbered %q is already waiting to be answered, so give this one a number of its own", shortenedText(id))
	}
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

// AskSecret asks for a secret on every attached screen, marked with the
// contract's own MaskInput flag so that the screen hides what is typed, and
// waits for the first screen to send it back.
// The secret goes straight to the caller: it is never written into the queue,
// never published on the event stream, and never sent back out to a screen.
//
// With no screen attached it returns contract.ErrNoMaskedPrompt, because there
// is then nowhere to type a secret where nobody can see it, which is what that
// error tells the caller to do something about.
func (socket *Socket) AskSecret(ctx context.Context, prompt string) (string, error) {
	id := socket.nextAskID()
	waiting := make(chan string, 1)
	socket.waitOnPrompt(id, waiting)
	defer socket.stopWaitingOnPrompt(id)

	asked, err := socket.writeToScreens(ctx, contract.SocketEnvelope{
		Type:      contract.SocketAsk,
		ID:        id,
		Text:      prompt,
		MaskInput: true,
	})
	if err != nil {
		return "", err
	}
	if asked == 0 {
		return "", fmt.Errorf("no screen is attached to type the secret into: %w", contract.ErrNoMaskedPrompt)
	}

	secret, answered := waitForAnswer(ctx, socket.options.Clock, socket.options.AnswerDeadline, waiting)
	if !answered {
		return "", fmt.Errorf("nobody typed the secret within %s, so ask again when you are at the terminal", socket.options.AnswerDeadline)
	}
	return secret, nil
}

// waitOnPrompt writes down that someone is waiting for a secret.
func (socket *Socket) waitOnPrompt(id string, waiting chan string) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	socket.prompts[id] = waiting
}

// stopWaitingOnPrompt forgets a masked prompt that has been answered or has run
// out of time, so that a late answer is told there is nothing to answer.
func (socket *Socket) stopWaitingOnPrompt(id string) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	delete(socket.prompts, id)
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

// waitOnPreview writes down that someone is waiting for an answer to a preview,
// and says no when that number is already taken. A number is what the user
// answers with, so two previews sharing one would leave one of them waiting for
// an answer that could never reach it.
func (socket *Socket) waitOnPreview(id string, waiting chan contract.PreviewAnswer) bool {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	if _, taken := socket.previews[id]; taken {
		return false
	}
	socket.previews[id] = waiting
	return true
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

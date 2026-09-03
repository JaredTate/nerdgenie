// The idea of showing what is about to happen as the message itself, with the
// answers explained in the same message, was borrowed from Hermes' Signal
// platform at ~/Code/hermes-agent/gateway/platforms/signal.py. The Go here is
// written fresh, and unlike the reference it waits for the answer on a clock the
// tests move rather than on the real one.

package signal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

const (
	// PreviewTimeout is how long a preview waits for an answer before it counts
	// as refused.
	PreviewTimeout = 30 * time.Minute
	// previewInstructions is the one line that explains the three answers, said
	// once with every preview and nowhere else.
	previewInstructions = "Reply \"approve\" to allow this once, \"always\" to allow every one like it, or \"deny\" to refuse."
)

// ShowPreview sends what is about to happen as the message itself, explains the
// three answers once, and waits for one of them.
func (channel *Channel) ShowPreview(ctx context.Context, preview contract.Preview) (contract.PreviewAnswerWithReason, error) {
	// The answer is waited for before the preview is sent, never after, because
	// somebody reading fast can reply before a send has finished returning.
	answers := make(chan contract.PreviewAnswer, 1)
	channel.guard.Lock()
	channel.waiting = answers
	channel.guard.Unlock()
	defer channel.forgetPreview(answers)

	shown := strings.TrimSpace(preview.Title + "\n\n" + preview.Body + "\n\n" + previewInstructions)
	if err := channel.client.Send(ctx, channel.recipient(), shown, nil); err != nil {
		return contract.PreviewAnswerWithReason{Answer: contract.AnswerReject}, err
	}

	waited, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	expired := make(chan struct{})
	go func() {
		if err := channel.clock.Sleep(waited, PreviewTimeout); err == nil {
			close(expired)
		}
	}()

	select {
	case answer := <-answers:
		return contract.PreviewAnswerWithReason{Answer: answer}, nil
	case <-expired:
		return contract.PreviewAnswerWithReason{Answer: contract.AnswerReject}, fmt.Errorf("nobody answered the preview %q within %v, so it counts as refused", preview.ID, PreviewTimeout)
	case <-ctx.Done():
		return contract.PreviewAnswerWithReason{Answer: contract.AnswerReject}, ctx.Err()
	}
}

// answerPreview hands one of the three words to whatever is waiting for it, and
// says whether the message was that answer rather than something for the agent.
func (channel *Channel) answerPreview(text string) bool {
	answer, isAnswer := readPreviewAnswer(text)
	if !isAnswer {
		return false
	}
	channel.guard.Lock()
	defer channel.guard.Unlock()
	if channel.waiting == nil {
		return false
	}
	select {
	case channel.waiting <- answer:
	default:
	}
	channel.waiting = nil
	return true
}

// readPreviewAnswer turns one of the three words into the answer it stands for.
func readPreviewAnswer(text string) (contract.PreviewAnswer, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "approve":
		return contract.AnswerOnce, true
	case "always":
		return contract.AnswerAlways, true
	case "deny":
		return contract.AnswerReject, true
	default:
		return "", false
	}
}

// forgetPreview takes the waiting answer away, unless something else is already
// waiting on a newer one.
func (channel *Channel) forgetPreview(answers chan contract.PreviewAnswer) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	if channel.waiting == answers {
		channel.waiting = nil
	}
}

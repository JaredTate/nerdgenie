package contract

import (
	"context"
	"errors"
	"time"
)

// Inbound is one message a user sent through a channel.
type Inbound struct {
	// ID is the channel's own identifier for the message.
	ID string
	// Sender identifies the person, in whatever form the channel uses.
	Sender string
	// Text is what they wrote.
	Text string
	// Attachments are the paths of any files or photos, already saved into the
	// inbox folder.
	Attachments []string
	// Received is when the channel saw it.
	Received time.Time
	// Channel is the name of the channel it came through.
	Channel string
}

// PreviewAnswer is how a user answers a preview or a permission question: this
// once, always for the session, or no with a reason.
type PreviewAnswer string

const (
	// AnswerOnce allows this one call and nothing more.
	AnswerOnce PreviewAnswer = "once"
	// AnswerAlways allows calls like this one for the rest of the session.
	AnswerAlways PreviewAnswer = "always"
	// AnswerReject refuses the call.
	AnswerReject PreviewAnswer = "reject"
)

// KnownPreviewAnswer says whether the answer is one of the three.
func KnownPreviewAnswer(answer PreviewAnswer) bool {
	switch answer {
	case AnswerOnce, AnswerAlways, AnswerReject:
		return true
	default:
		return false
	}
}

// PreviewAnswerWithReason is how a user answered a preview, with the reason
// they gave when they refused, in their own words, so the model is told why.
type PreviewAnswerWithReason struct {
	// Answer is once, always, or reject.
	Answer PreviewAnswer
	// Reason is the user's words on a reject, and empty otherwise.
	Reason string
}

// Preview is exactly what is about to happen, shown to the user before it does.
type Preview struct {
	// ID is what the user answers with, as in "/approve 3".
	ID string
	// Title is one line saying what kind of thing this is.
	Title string
	// Body is the actual text of the command, the post, or the file change.
	Body string
}

// ChannelHealth says whether a channel can carry messages right now.
type ChannelHealth struct {
	// Healthy is true when the channel is working.
	Healthy bool
	// Detail says what is wrong when it is not, in plain words.
	Detail string
}

// Channel is anything a user can talk through. Every channel does the same six
// things, so the agent loop never knows which one it is talking to.
type Channel interface {
	// Name is the channel's name, such as "terminal" or "signal".
	Name() string
	// Receive returns the stream of inbound messages. The stream closes when the
	// context is cancelled or the channel shuts down.
	Receive(ctx context.Context) (<-chan Inbound, error)
	// Send delivers one reply.
	Send(ctx context.Context, text string) error
	// SendFile delivers one file with a caption.
	SendFile(ctx context.Context, path string, caption string) error
	// ShowPreview shows what is about to happen and waits for the answer.
	ShowPreview(ctx context.Context, preview Preview) (PreviewAnswerWithReason, error)
	// AskSecret prompts for a secret without echoing it, or returns
	// ErrNoMaskedPrompt when the channel cannot hide what is typed.
	AskSecret(ctx context.Context, prompt string) (string, error)
	// Health says whether the channel is working.
	Health(ctx context.Context) ChannelHealth
}

// ErrNoMaskedPrompt means the channel cannot hide what the user types, so it
// must never be used for a secret. Signal returns it; the terminal does not.
var ErrNoMaskedPrompt = errors.New("this channel cannot hide what you type, so enter the secret in the terminal instead")

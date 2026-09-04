package testkit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// inboundQueueSize caps how many messages a test may push before anything reads
// them, because every buffer in Coeus has a cap.
const inboundQueueSize = 64

// SentFile is one file the harness sent through a channel.
type SentFile struct {
	// Path is the file that was sent.
	Path string
	// Caption is the line that went with it.
	Caption string
}

// FakeChannel stands in for the terminal or Signal: a test pushes messages in,
// the harness sends replies out, and the fake records everything so the test can
// say what the user would have seen.
type FakeChannel struct {
	guard        sync.Mutex
	name         string
	inbound      chan contract.Inbound
	stopped      chan struct{}
	shutDown     bool
	sent         []string
	files        []SentFile
	previews     []contract.Preview
	answer       contract.PreviewAnswer
	reason       string
	secret       string
	cannotMask   bool
	healthy      bool
	healthDetail string
}

// NewFakeChannel returns a channel that is healthy, answers every preview with
// "once", and gives back an empty secret until a test says otherwise.
func NewFakeChannel(name string) *FakeChannel {
	return &FakeChannel{
		name:    name,
		inbound: make(chan contract.Inbound, inboundQueueSize),
		stopped: make(chan struct{}),
		answer:  contract.AnswerOnce,
		healthy: true,
	}
}

// Push puts one message into the channel as though a user sent it, and reports
// the overflow rather than blocking when the queue is full and nothing is
// reading it.
func (channel *FakeChannel) Push(message contract.Inbound) error {
	if message.Channel == "" {
		message.Channel = channel.name
	}
	if message.Received.IsZero() {
		message.Received = time.Unix(0, 0).UTC()
	}
	select {
	case channel.inbound <- message:
		return nil
	default:
		return fmt.Errorf("the fake channel %q is already holding %d messages nobody has read, so read some before pushing more",
			channel.name, inboundQueueSize)
	}
}

// Shutdown closes every stream the channel handed out, the way a channel that is
// going away does. Calling it twice is harmless.
func (channel *FakeChannel) Shutdown() {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	if channel.shutDown {
		return
	}
	channel.shutDown = true
	close(channel.stopped)
}

// Sent is every reply the harness sent, in order.
func (channel *FakeChannel) Sent() []string {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	copied := make([]string, len(channel.sent))
	copy(copied, channel.sent)
	return copied
}

// Files is every file the harness sent, in order.
func (channel *FakeChannel) Files() []SentFile {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	copied := make([]SentFile, len(channel.files))
	copy(copied, channel.files)
	return copied
}

// Previews is every preview the harness showed, in order.
func (channel *FakeChannel) Previews() []contract.Preview {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	copied := make([]contract.Preview, len(channel.previews))
	copy(copied, channel.previews)
	return copied
}

// AnswerPreviewsWith says how the user answers every preview from now on.
func (channel *FakeChannel) AnswerPreviewsWith(answer contract.PreviewAnswer) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.answer = answer
}

// AnswerSecretWith says what the user types at a masked prompt.
func (channel *FakeChannel) AnswerSecretWith(secret string) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.secret = secret
}

// CannotMaskSecrets makes the channel refuse a masked prompt, the way Signal
// does, because Signal cannot hide what the user types.
func (channel *FakeChannel) CannotMaskSecrets() {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.cannotMask = true
}

// SetHealth says whether the channel is working, and why not when it is not.
func (channel *FakeChannel) SetHealth(healthy bool, detail string) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.healthy, channel.healthDetail = healthy, detail
}

// Name is the channel's name.
func (channel *FakeChannel) Name() string {
	return channel.name
}

// Receive returns a stream of its own, fed from the queue a test pushes into.
// The stream closes when the context is cancelled or the channel shuts down,
// which is what contract.Channel promises and what a harness that leaks an
// attach depends on.
func (channel *FakeChannel) Receive(ctx context.Context) (<-chan contract.Inbound, error) {
	stream := make(chan contract.Inbound)
	go channel.feed(ctx, stream)
	return stream, nil
}

// feed moves messages from the queue onto one attached stream until the context
// is cancelled or the channel shuts down, and closes the stream on its way out.
func (channel *FakeChannel) feed(ctx context.Context, stream chan contract.Inbound) {
	defer close(stream)
	for {
		var message contract.Inbound
		select {
		case <-ctx.Done():
			return
		case <-channel.stopped:
			return
		case message = <-channel.inbound:
		}

		select {
		case stream <- message:
		case <-ctx.Done():
			return
		case <-channel.stopped:
			return
		}
	}
}

// Send records one reply.
func (channel *FakeChannel) Send(_ context.Context, text string) error {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.sent = append(channel.sent, text)
	return nil
}

// SendFile records one file with its caption.
func (channel *FakeChannel) SendFile(_ context.Context, path string, caption string) error {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.files = append(channel.files, SentFile{Path: path, Caption: caption})
	return nil
}

// ShowPreview records the preview and answers it the way the test said.
func (channel *FakeChannel) ShowPreview(_ context.Context, preview contract.Preview) (contract.PreviewAnswerWithReason, error) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.previews = append(channel.previews, preview)
	return contract.PreviewAnswerWithReason{Answer: channel.answer, Reason: channel.reason}, nil
}

// AnswerPreviewsWithReason makes every later preview come back refused with the
// user's reason, which is what a test of the reject path needs.
func (channel *FakeChannel) AnswerPreviewsWithReason(reason string) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.answer = contract.AnswerReject
	channel.reason = reason
}

// AskSecret gives back what the test set, or refuses when the channel cannot
// hide what is typed.
func (channel *FakeChannel) AskSecret(_ context.Context, _ string) (string, error) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	if channel.cannotMask {
		return "", contract.ErrNoMaskedPrompt
	}
	return channel.secret, nil
}

// Health says whether the channel is working.
func (channel *FakeChannel) Health(_ context.Context) contract.ChannelHealth {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	return contract.ChannelHealth{Healthy: channel.healthy, Detail: channel.healthDetail}
}

package replay

import (
	"context"

	"github.com/JaredTate/coeus/internal/contract"
)

// quietChannel is the channel a replay answers on. Nobody is reading it, so
// everything it is given goes nowhere.
//
// It approves whatever it is shown. That is safe here and nowhere else: in a
// replay no tool touches the machine, because every one of them hands back the
// text the log says it returned. What is under test is whether the permission
// function still rules the same way, and a preview that could never be answered
// would end every replay of a task the user once approved.
type quietChannel struct {
	// name is the channel the recorded ask came in on, so that the replayed
	// record's header says what the recorded one said.
	name string
}

// Name is the channel the recorded ask came in on.
func (quiet quietChannel) Name() string {
	return quiet.name
}

// Receive hands back a stream that is closed at once, because a replay takes no
// messages from anybody.
func (quiet quietChannel) Receive(_ context.Context) (<-chan contract.Inbound, error) {
	stream := make(chan contract.Inbound)
	close(stream)
	return stream, nil
}

// Send throws the reply away, because a replay has no reader.
func (quiet quietChannel) Send(_ context.Context, _ string) error {
	return nil
}

// SendFile throws the file away for the same reason.
func (quiet quietChannel) SendFile(_ context.Context, _ string, _ string) error {
	return nil
}

// ShowPreview approves what it is shown, because nothing in a replay reaches
// the machine and a preview nobody can answer would end the run.
func (quiet quietChannel) ShowPreview(_ context.Context, _ contract.Preview) (contract.PreviewAnswerWithReason, error) {
	return contract.PreviewAnswerWithReason{Answer: contract.AnswerOnce}, nil
}

// AskSecret refuses, because a replay has nobody to type a secret and must
// never be a way of getting one out of the vault.
func (quiet quietChannel) AskSecret(_ context.Context, _ string) (string, error) {
	return "", contract.ErrNoMaskedPrompt
}

// Health says the channel is working, because there is nothing in it to break.
func (quiet quietChannel) Health(_ context.Context) contract.ChannelHealth {
	return contract.ChannelHealth{Healthy: true}
}

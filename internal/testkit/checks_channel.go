package testkit

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// CheckChannel asserts what every channel promises: it has a name, it can be
// attached to, it takes a reply and a file, a preview comes back as one of the
// three answers, a masked prompt either works or says it cannot, and it reports
// its health.
func CheckChannel(ctx context.Context, channel contract.Channel) error {
	if channel.Name() == "" {
		return errors.New("the channel has no name, and the router prints it on every message")
	}

	attached, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := channel.Receive(attached); err != nil {
		return fmt.Errorf("attaching to the channel failed: %w", err)
	}

	if err := channel.Send(ctx, "the contract check sent this"); err != nil {
		return fmt.Errorf("sending a reply failed: %w", err)
	}
	if err := channel.SendFile(ctx, "/tmp/contract-check.png", "sent by the contract check"); err != nil {
		return fmt.Errorf("sending a file failed: %w", err)
	}

	answer, err := channel.ShowPreview(ctx, contract.Preview{
		ID:    "contract-check",
		Title: "the contract check",
		Body:  "nothing is about to happen",
	})
	if err != nil {
		return fmt.Errorf("showing a preview failed: %w", err)
	}
	if !contract.KnownPreviewAnswer(answer) {
		return fmt.Errorf("the preview was answered %q, want once, always, or reject", answer)
	}

	if _, err := channel.AskSecret(ctx, "a secret for the contract check"); err != nil && !errors.Is(err, contract.ErrNoMaskedPrompt) {
		return fmt.Errorf("the masked prompt failed with something other than ErrNoMaskedPrompt: %w", err)
	}

	health := channel.Health(ctx)
	if !health.Healthy && health.Detail == "" {
		return errors.New("the channel says it is unhealthy and does not say why, and the user has to be told what to fix")
	}
	return nil
}

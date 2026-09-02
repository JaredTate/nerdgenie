package signal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// PairCommand builds the "/pair" command, which approves the sender whose
// pairing code matches the one typed. The orchestrator registers it in serve.go.
//
// The command is not marked terminal only, because the rule is not that simple:
// while nobody at all is paired there is nobody over Signal who could be trusted
// to pair anybody, so the first pairing has to come from the terminal. Once one
// sender is paired, that sender may pair the next one from their phone.
func PairCommand(pairing *Pairing) contract.Command {
	return contract.Command{
		Name: "pair",
		Help: "Approves the sender whose pairing code you type, as in \"/pair ABCD2345\".",
		Run: func(_ context.Context, arguments string, where contract.CommandContext) (string, error) {
			if err := allowedHere(pairing, where); err != nil {
				return "", err
			}
			typed := strings.TrimSpace(arguments)
			if typed == "" {
				return "Type \"/pair\" and the eight-character code the sender was given, as in \"/pair ABCD2345\".", nil
			}
			sender, err := pairing.Approve(typed)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Paired %s. They can talk to me over Signal now.", sender), nil
		},
	}
}

// allowedHere holds the rule that the first pairing comes from the terminal.
func allowedHere(pairing *Pairing, where contract.CommandContext) error {
	if where.Channel != nil && where.Channel.Name() == contract.TerminalChannelName {
		return nil
	}
	if pairing.HasApproved() {
		return nil
	}
	return errors.New("nobody is paired yet, so run the pairing command in the terminal on the machine Coeus runs on")
}

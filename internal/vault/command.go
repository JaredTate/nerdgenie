package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// terminalChannelName is the name of the channel a person types into directly.
// The vault answers on that channel and on no other, because Signal cannot hide
// what is typed and a secret must never travel through a message.
const terminalChannelName = "terminal"

// The two lines the command answers with when it cannot do what was asked.
const (
	onlyInTheTerminal = "the vault works only in the terminal, so open a terminal on the machine Coeus runs on and try there"
	vaultUsage        = "the vault understands four words: list, add <name>, remove <name>, and test <name>"
)

// NewCommand returns the /vault slash command for one vault. The orchestrator
// registers the value this returns; the vault it is built with is the one the
// running program opened.
func NewCommand(store *Vault) contract.Command {
	return contract.Command{
		Name:         "vault",
		Help:         "Lists, adds, removes, and tests the secrets Coeus holds. Terminal only.",
		TerminalOnly: true,
		Run: func(ctx context.Context, arguments string, where contract.CommandContext) (string, error) {
			return runVault(ctx, store, arguments, where)
		},
	}
}

// runVault does the work of the slash command.
func runVault(ctx context.Context, store *Vault, arguments string, where contract.CommandContext) (string, error) {
	if where.Channel == nil || where.Channel.Name() != terminalChannelName {
		return onlyInTheTerminal, nil
	}

	word, name := splitFirstWord(arguments)
	switch word {
	case "", "list":
		return listing(store), nil
	case "add":
		return addEntry(ctx, store, name, where.Channel)
	case "remove":
		return removeEntry(store, name)
	case "test":
		return testEntry(store, name)
	default:
		return vaultUsage, nil
	}
}

// listing is the names and sites of everything the vault holds, and never a
// value.
func listing(store *Vault) string {
	held := store.List()
	if len(held) == 0 {
		return "the vault is empty, so add a login with /vault add <name> in the terminal"
	}

	lines := make([]string, 0, len(held)+1)
	lines = append(lines, fmt.Sprintf("the vault holds %d entries:", len(held)))
	for _, one := range held {
		if one.Site == "" {
			lines = append(lines, "  "+one.Name)
			continue
		}
		lines = append(lines, "  "+one.Name+" - "+one.Site)
	}
	return strings.Join(lines, "\n")
}

// addEntry asks for every value through the channel's masked prompt and stores
// them. Nothing that was typed is printed, logged, or sent anywhere.
func addEntry(ctx context.Context, store *Vault, name string, channel contract.Channel) (string, error) {
	if name == "" {
		return vaultUsage, nil
	}

	entry, err := askForEntry(ctx, channel, name)
	if errors.Is(err, contract.ErrNoMaskedPrompt) {
		return contract.ErrNoMaskedPrompt.Error(), nil
	}
	if err != nil {
		return "", err
	}
	if err := store.Add(entry); err != nil {
		return "", err
	}
	return fmt.Sprintf("the entry %q is in the vault, and nothing you typed was printed or logged", name), nil
}

// questionsFor is what the masked prompt asks for, in order. The sudo entry is
// only a password, because it is the machine's password and not a login.
func questionsFor(name string) []string {
	if name == SudoEntryName {
		return []string{"the sudo password for this machine"}
	}
	return []string{
		"the site this login is for, such as X",
		"the hostnames it may be typed into, separated by commas",
		"the login name",
		"the password",
		"the two-factor secret, or nothing if the site has none",
	}
}

// askForEntry puts every question to the user through the masked prompt.
func askForEntry(ctx context.Context, channel contract.Channel, name string) (Entry, error) {
	questions := questionsFor(name)
	answers := make([]string, 0, len(questions))
	for _, question := range questions {
		typed, err := channel.AskSecret(ctx, "Enter "+question+": ")
		if err != nil {
			return Entry{}, err
		}
		answers = append(answers, strings.TrimSpace(typed))
	}

	if name == SudoEntryName {
		return Entry{Name: name, Password: answers[0]}, nil
	}
	return Entry{
		Name:       name,
		Site:       answers[0],
		Domains:    splitDomains(answers[1]),
		Username:   answers[2],
		Password:   answers[3],
		TOTPSecret: answers[4],
	}, nil
}

// splitDomains turns a comma-separated answer into the hostnames it names.
func splitDomains(answer string) []string {
	found := []string{}
	for _, part := range strings.Split(answer, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			found = append(found, trimmed)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

// removeEntry takes one entry out of the vault.
func removeEntry(store *Vault, name string) (string, error) {
	if name == "" {
		return vaultUsage, nil
	}
	if err := store.Remove(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("the entry %q is out of the vault", name), nil
}

// testEntry says whether one entry still decrypts and still makes a code.
func testEntry(store *Vault, name string) (string, error) {
	if name == "" {
		return vaultUsage, nil
	}
	return store.Check(name)
}

// Check reads one entry back out of the file on disk and says whether it
// decrypts and whether a two-factor code can still be made from it. Neither the
// values nor the code are ever printed.
func (vault *Vault) Check(name string) (string, error) {
	vault.guard.Lock()
	identity, path := vault.identity, vault.home.VaultFile()
	closed := vault.closed
	vault.guard.Unlock()
	if closed {
		return "", errors.New("the vault is closed, so open it again before testing an entry")
	}

	held, err := readVaultFile(path, identity)
	if err != nil {
		return "", err
	}
	at := slices.IndexFunc(held, func(one Entry) bool { return one.Name == name })
	if at < 0 {
		return "", fmt.Errorf("the vault file holds no entry named %q, so run /vault list to see what it does hold", name)
	}
	if held[at].TOTPSecret == "" {
		return fmt.Sprintf("the entry %q decrypts, and it has no two-factor secret", name), nil
	}
	if _, _, err := vault.Code(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("the entry %q decrypts, and a two-factor code was made from it, which is never printed", name), nil
}

// splitFirstWord splits a command's arguments into the first word and the rest.
func splitFirstWord(arguments string) (string, string) {
	trimmed := strings.TrimSpace(arguments)
	at := strings.IndexAny(trimmed, " \t")
	if at < 0 {
		return trimmed, ""
	}
	return trimmed[:at], strings.TrimSpace(trimmed[at+1:])
}

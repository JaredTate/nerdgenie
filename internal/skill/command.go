package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// Command returns the "/skills" slash command, which the orchestrator registers
// in serve.go. On its own it lists what there is; "show" prints one skill with
// its changelog; "run" replays one; "rollback" puts one back to the copy kept
// beside it; and "remove" takes one out of use without throwing it away.
func (store *Store) Command() contract.Command {
	return contract.Command{
		Name: "skills",
		Help: "list the skills, /skills show|run|rollback|remove <name>",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			return store.runCommand(ctx, arguments)
		},
	}
}

// runCommand reads the words after "/skills" and does what they ask for.
func (store *Store) runCommand(ctx context.Context, arguments string) (string, error) {
	word, rest, _ := strings.Cut(strings.TrimSpace(arguments), " ")
	rest = strings.TrimSpace(rest)
	switch word {
	case "":
		return store.showTheList(ctx)
	case "show":
		return store.showOneSkill(ctx, rest)
	case "run":
		return store.runOneSkill(ctx, rest)
	case "rollback":
		return store.Rollback(ctx, rest)
	case "remove":
		return store.Remove(ctx, rest)
	default:
		return "", fmt.Errorf("skills does not know the word %q, so use /skills, or /skills show, run, rollback, or remove with a name", word)
	}
}

// showTheList prints every skill's name and one-liner, which is the same pair
// the model sees in its prompt.
func (store *Store) showTheList(ctx context.Context) (string, error) {
	listed, err := store.List(ctx)
	if err != nil {
		return "", err
	}
	if len(listed) == 0 {
		return "There are no skills yet. One is written when you point the agent at a page of documentation, or when it offers to save a task it has just finished.", nil
	}
	lines := []string{fmt.Sprintf("There are %d skills:", len(listed))}
	for _, summary := range listed {
		lines = append(lines, fmt.Sprintf("- %s: %s", summary.Name, summary.Description))
	}
	return strings.Join(lines, "\n"), nil
}

// showOneSkill prints one skill's body and its changelog.
func (store *Store) showOneSkill(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("skills needs to know which skill to show, so write /skills show and the name of one")
	}
	body, err := store.Load(ctx, name)
	if err != nil {
		return "", err
	}
	changelog, err := store.Changelog(name)
	if err != nil {
		return "", err
	}
	return body + "\n" + changelog, nil
}

// runOneSkill replays one skill and reports what each step did.
func (store *Store) runOneSkill(ctx context.Context, arguments string) (string, error) {
	name, rest, _ := strings.Cut(arguments, " ")
	if name == "" {
		return "", fmt.Errorf("skills needs to know which skill to run, so write /skills run and the name of one")
	}
	report, err := store.RunForPerson(ctx, name, strings.TrimSpace(rest))
	if err != nil {
		return report, err
	}
	if strings.TrimSpace(report) == "" {
		return fmt.Sprintf("The skill %q ran and had nothing to report.", name), nil
	}
	return report, nil
}

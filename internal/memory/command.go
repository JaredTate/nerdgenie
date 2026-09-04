package memory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// commandFactsShown is how many facts the "/memory" listing prints, which is
// enough to see what the agent has learned lately without filling a screen.
const commandFactsShown = 10

// Command returns the "/memory" slash command, which the orchestrator registers
// in serve.go. On its own it shows how full the two files are and the facts
// written down most recently; "search" looks something up; and "forget"
// withdraws a fact without deleting it.
func (memory *Memory) Command() contract.Command {
	return contract.Command{
		Name: "memory",
		Help: "show what is remembered, /memory search <words>, /memory forget <id>",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			return memory.runCommand(ctx, arguments)
		},
	}
}

// runCommand reads the words after "/memory" and does what they ask for.
func (memory *Memory) runCommand(ctx context.Context, arguments string) (string, error) {
	word, rest, _ := strings.Cut(strings.TrimSpace(arguments), " ")
	rest = strings.TrimSpace(rest)
	switch word {
	case "":
		return memory.showWhatIsRemembered(ctx)
	case "search":
		return memory.showWhatMatches(ctx, rest)
	case "forget":
		return memory.withdrawFact(ctx, rest)
	default:
		return "", fmt.Errorf("memory does not know the word %q, so use /memory, /memory search <words>, or /memory forget <id>", word)
	}
}

// showWhatIsRemembered prints how full the two files are and the facts written
// down most recently.
func (memory *Memory) showWhatIsRemembered(ctx context.Context) (string, error) {
	lines := []string{fmt.Sprintf("MEMORY.md holds %d of %d bytes, and USER.md holds %d of %d bytes.",
		sizeOnDisk(memory.home.WorldFactsFile()), memory.caps.WorldFactsBytes,
		sizeOnDisk(memory.home.UserFactsFile()), memory.caps.UserFactsBytes)}

	facts, err := memory.latestFacts(ctx, commandFactsShown)
	if err != nil {
		return "", err
	}
	if len(facts) == 0 {
		return lines[0] + "\nMemory holds no facts yet.", nil
	}
	lines = append(lines, fmt.Sprintf("The last %d facts, newest first:", len(facts)))
	for _, fact := range facts {
		lines = append(lines, formatFactLine(fact))
	}
	return strings.Join(lines, "\n"), nil
}

// showWhatMatches prints what a search for some words found.
func (memory *Memory) showWhatMatches(ctx context.Context, words string) (string, error) {
	if words == "" {
		return "", fmt.Errorf("memory needs something to search for, so write /memory search %s", "<words>")
	}
	found, err := memory.Search(ctx, words, commandFactsShown)
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return fmt.Sprintf("Nothing in memory matches %q.", words), nil
	}
	lines := []string{fmt.Sprintf("%d results for %q, the best match first:", len(found), words)}
	for _, fact := range found {
		lines = append(lines, formatFactLine(fact))
	}
	return strings.Join(lines, "\n"), nil
}

// withdrawFact supersedes a fact with a note saying the user withdrew it. The
// old fact is not deleted: it stays searchable and every result says that
// something later replaced it. The note names the withdrawn fact and never
// copies its words, because a copy would be a live fact holding exactly what the
// user asked the agent to stop repeating.
func (memory *Memory) withdrawFact(ctx context.Context, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("memory needs to know which fact to forget, so write /memory forget %s", "<id>")
	}
	if _, _, err := factRow(ctx, memory.database, id); err != nil {
		return "", err
	}
	err := memory.Save(ctx, []contract.Fact{{
		Text:       "the user withdrew the fact " + id + ", so what it said is no longer true",
		Source:     "the user",
		Supersedes: id,
	}})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s is withdrawn. It is not deleted: it stays searchable, and every result now says it was superseded.", id), nil
}

// latestFacts reads the facts written down most recently, newest first.
func (memory *Memory) latestFacts(ctx context.Context, limit int) ([]contract.Fact, error) {
	rows, err := memory.database.QueryContext(ctx,
		`SELECT id, text, source, recorded, supersedes, superseded_by FROM memory_facts
		 ORDER BY recorded DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot read the facts memory holds: %w", err)
	}
	defer rows.Close()

	facts := []contract.Fact{}
	for rows.Next() {
		fact := contract.Fact{}
		recorded, supersededBy := "", ""
		if err := rows.Scan(&fact.ID, &fact.Text, &fact.Source, &recorded, &fact.Supersedes, &supersededBy); err != nil {
			return nil, fmt.Errorf("cannot read one of the facts memory holds: %w", err)
		}
		fact.Recorded = fromStoredTime(recorded)
		fact.Text = markIfSuperseded(fact.Text, supersededBy)
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the facts memory holds: %w", err)
	}
	return facts, nil
}

// sizeOnDisk is how many bytes a memory file holds, and zero when it is not
// there yet.
func sizeOnDisk(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

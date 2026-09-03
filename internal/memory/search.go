// What one search covers is the design of OpenClaw's memory tool contract at
// ~/Code/openclaw/extensions/memory-core/src/memory-tool-contract.ts, written
// fresh in Go: search over MEMORY.md, USER.md, the markdown files under the
// memory folder, and the indexed past conversations, with a second call that
// reads one of them back in full by the name the search gave it. OpenClaw makes
// the corpus a parameter the model has to choose; Coeus searches all of it at
// once and says in the id of each result which kind of thing it is, because a
// model that has to pick a corpus first will pick the wrong one.

package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxSearchResults is the most results one search hands back, whatever limit it
// was asked for, because a search result rides in the model's context.
const MaxSearchResults = 50

// MaxHintRunes is the longest one line of the memory hint may be. Three lines
// of this length is what rides below the cache line on every turn.
const MaxHintRunes = 120

// maxSnippetRunes is how much of a note or a past message a search result
// shows. The whole of it comes back from Get.
const maxSnippetRunes = 300

// The bounds on a query. A step's text can be long, and every word of it would
// otherwise become another term the search table has to weigh.
const (
	maxQueryTokens = 32
	maxTokenRunes  = 64
)

// The columns and tables every search reads, written out in full once. The join
// to the fact table is on the left, so that a note or a past message comes back
// with empty fact columns rather than not at all.
const (
	searchColumns = `SELECT indexed.kind, indexed.reference, indexed.recorded, indexed.source,
		memory_search.body, COALESCE(facts.text, ''), COALESCE(facts.supersedes, ''),
		COALESCE(facts.superseded_by, '')`
	searchTables = ` FROM memory_search
		JOIN memory_indexed AS indexed ON indexed.row_id = memory_search.rowid
		LEFT JOIN memory_facts AS facts ON indexed.kind = 'fact' AND facts.id = indexed.reference`
)

// Search finds what matches a query: the best match first and, among matches
// the search table scores the same, the newest first. A query with no words in
// it at all matches everything, and then the newest come back first. Facts,
// notes, and past messages all come back as facts, and a fact something later
// replaced is marked as superseded in its own text.
func (memory *Memory) Search(ctx context.Context, query string, limit int) ([]contract.Fact, error) {
	if limit <= 0 || limit > MaxSearchResults {
		limit = MaxSearchResults
	}
	expression := matchExpression(query)
	statement := searchColumns + searchTables +
		" WHERE memory_search MATCH ? ORDER BY bm25(memory_search), indexed.recorded DESC LIMIT ?"
	arguments := []any{expression, limit}
	if expression == "" {
		statement = searchColumns + searchTables +
			" ORDER BY indexed.recorded DESC, indexed.row_id DESC LIMIT ?"
		arguments = []any{limit}
	}

	rows, err := memory.database.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("cannot search memory for %q: %w", query, err)
	}
	defer rows.Close()
	return readSearchHits(rows, query)
}

// readSearchHits turns the rows of a search into facts.
func readSearchHits(rows *sql.Rows, query string) ([]contract.Fact, error) {
	found := []contract.Fact{}
	for rows.Next() {
		hit := searchHit{}
		if err := rows.Scan(&hit.kind, &hit.reference, &hit.recorded, &hit.source,
			&hit.body, &hit.text, &hit.supersedes, &hit.supersededBy); err != nil {
			return nil, fmt.Errorf("cannot read a result of the memory search for %q: %w", query, err)
		}
		found = append(found, hit.asFact())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish the memory search for %q: %w", query, err)
	}
	return found, nil
}

// searchHit is one row of a search, before it is turned into a fact.
type searchHit struct {
	kind         string
	reference    string
	recorded     string
	source       string
	body         string
	text         string
	supersedes   string
	supersededBy string
}

// asFact turns one row of a search into the fact a caller sees.
func (hit searchHit) asFact() contract.Fact {
	fact := contract.Fact{
		Source:   hit.source,
		Recorded: fromStoredTime(hit.recorded),
	}
	switch hit.kind {
	case factEntry:
		fact.ID = hit.reference
		fact.Text = markIfSuperseded(hit.text, hit.supersededBy)
		fact.Supersedes = hit.supersedes
	case noteEntry:
		fact.ID = NoteIDPrefix + hit.reference
		fact.Text = cutToRunes(oneLine(hit.body), maxSnippetRunes)
	default:
		fact.ID = MessageIDPrefix + hit.reference
		fact.Text = cutToRunes(oneLine(hit.body), maxSnippetRunes)
	}
	return fact
}

// markIfSuperseded says in the fact's own words that something later replaced
// it, because nothing is ever deleted and a reader has to be told.
func markIfSuperseded(text string, supersededBy string) string {
	if supersededBy == "" {
		return text
	}
	return text + " (superseded by " + supersededBy + ")"
}

// Get returns one thing the index holds by its id: a fact by its own id, a note
// by "note:" and its path inside the home folder, or a past message by "msg:"
// and its number in the event log.
func (memory *Memory) Get(ctx context.Context, id string) (contract.Fact, error) {
	if reference, isNote := strings.CutPrefix(id, NoteIDPrefix); isNote {
		return memory.getNote(ctx, id, reference)
	}
	if reference, isMessage := strings.CutPrefix(id, MessageIDPrefix); isMessage {
		return memory.getMessage(ctx, id, reference)
	}
	fact, supersededBy, err := factRow(ctx, memory.database, id)
	if err != nil {
		return contract.Fact{}, err
	}
	fact.Text = markIfSuperseded(fact.Text, supersededBy)
	return fact, nil
}

// getNote reads one note out of the memory folder in full, up to the size a
// note may be.
func (memory *Memory) getNote(ctx context.Context, id string, reference string) (contract.Fact, error) {
	fact, err := memory.indexedEntry(ctx, noteEntry, reference, id)
	if err != nil {
		return contract.Fact{}, err
	}
	path, err := memory.pathInsideTheHome(reference)
	if err != nil {
		return contract.Fact{}, err
	}
	held, err := readWholeFile(path)
	if err != nil {
		return contract.Fact{}, err
	}
	fact.Text = cutToRunes(string(held), MaxNoteBytes)
	return fact, nil
}

// getMessage reads one past message back out of the event log.
func (memory *Memory) getMessage(ctx context.Context, id string, reference string) (contract.Fact, error) {
	fact, err := memory.indexedEntry(ctx, messageEntry, reference, id)
	if err != nil {
		return contract.Fact{}, err
	}
	sequence, err := strconv.ParseInt(reference, 10, 64)
	if err != nil {
		return contract.Fact{}, fmt.Errorf("the id %q does not name a message in the event log, so use the id a search gave you", id)
	}
	event, err := memory.eventLog.ByID(ctx, sequence)
	if err != nil {
		return contract.Fact{}, fmt.Errorf("cannot read the message %q back out of the event log: %w", id, err)
	}
	body := messageBody{}
	if err := json.Unmarshal(event.Body, &body); err != nil {
		return contract.Fact{}, fmt.Errorf("cannot read what the message %q said: %w", id, err)
	}
	fact.Text = oneLine(body.Text)
	return fact, nil
}

// indexedEntry reads the index's own record of a note or a past message, which
// is where its date and its source come from.
func (memory *Memory) indexedEntry(ctx context.Context, kind string, reference string, id string) (contract.Fact, error) {
	recorded, source := "", ""
	row := memory.database.QueryRowContext(ctx,
		"SELECT recorded, source FROM memory_indexed WHERE kind = ? AND reference = ?", kind, reference)
	switch err := row.Scan(&recorded, &source); {
	case errors.Is(err, sql.ErrNoRows):
		return contract.Fact{}, fmt.Errorf("memory holds nothing with the id %q, so search for it instead", id)
	case err != nil:
		return contract.Fact{}, fmt.Errorf("cannot read the %s %q out of the memory index: %w", kind, id, err)
	}
	return contract.Fact{ID: id, Source: source, Recorded: fromStoredTime(recorded)}, nil
}

// pathInsideTheHome turns a reference the index holds back into a path, and
// refuses one that would lead out of the home folder.
func (memory *Memory) pathInsideTheHome(reference string) (string, error) {
	path := filepath.Clean(filepath.Join(memory.home.Root, filepath.FromSlash(reference)))
	if path != memory.home.Root && !strings.HasPrefix(path, memory.home.Root+string(os.PathSeparator)) {
		return "", fmt.Errorf("the reference %q leads out of the home folder, so it is not something memory can read", reference)
	}
	return path, nil
}

// Hint returns at most three short lines for the end of the prompt, chosen by
// searching for the text of the step the agent is on, and nothing at all when
// nothing matches or when there is nothing to search for.
func (memory *Memory) Hint(ctx context.Context, query string) ([]string, error) {
	if matchExpression(query) == "" {
		return nil, nil
	}
	found, err := memory.Search(ctx, query, contract.MemoryHintLines)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	lines := make([]string, 0, len(found))
	for _, fact := range found {
		lines = append(lines, cutToRunes(oneLine(fact.Text), MaxHintRunes))
	}
	return lines, nil
}

// matchExpression turns a query into the expression the search table matches
// on: every distinct word, quoted so that a word the search table treats as one
// of its own is read as a word, and joined so that any of them counts as a
// match. A query with no words in it returns nothing at all.
func matchExpression(query string) string {
	seen := map[string]bool{}
	terms := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(query), notPartOfAWord) {
		if len([]rune(word)) > maxTokenRunes {
			word = string([]rune(word)[:maxTokenRunes])
		}
		if seen[word] {
			continue
		}
		seen[word] = true
		terms = append(terms, `"`+word+`"`)
		if len(terms) >= maxQueryTokens {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

// notPartOfAWord says where one word of a query ends, which is the same place
// the search table's own tokenizer ends one.
func notPartOfAWord(letter rune) bool {
	return !unicode.IsLetter(letter) && !unicode.IsDigit(letter)
}

// cutToRunes shortens text to a number of runes, saying with an ellipsis that
// it was cut.
func cutToRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit || limit < 1 {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

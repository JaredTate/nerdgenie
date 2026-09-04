package memory

import (
	"context"
	"slices"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxHintRunes is the longest one line of the memory hint may be. Three lines of
// this length is what rides below the cache line on every turn.
const maxHintRunes = 120

// minimumHintWordRunes is the shortest word of a step the hint will search for.
// Every word below it is a joining word rather than a subject, so searching for
// it matches whatever happens to be in memory and says nothing.
const minimumHintWordRunes = 4

// hintWordsThatMustMatch is how many of the step's own words a line has to hold
// before it is worth a place in the prompt. A line sharing one word with a step
// is usually sharing an accident; a line sharing two is usually about the same
// thing. A step that offers only one word of its own is answered on that one.
const hintWordsThatMustMatch = 2

// maxHintCandidates is how many results the hint scores before it picks its
// three, so that the search table's best answers are all looked at without the
// whole index being read on every turn.
const maxHintCandidates = 20

// hintStopWords are the words a step is not searched on, because every other
// sentence holds them too. Every word shorter than minimumHintWordRunes is
// already dropped, so this list only has to name the longer ones.
var hintStopWords = map[string]bool{
	"about": true, "above": true, "after": true, "again": true, "against": true,
	"along": true, "already": true, "also": true, "although": true, "always": true,
	"another": true, "anything": true, "because": true, "been": true, "before": true,
	"being": true, "below": true, "between": true, "both": true, "cannot": true,
	"could": true, "does": true, "doing": true, "done": true, "down": true,
	"during": true, "each": true, "either": true, "else": true, "even": true,
	"ever": true, "every": true, "from": true, "further": true, "have": true,
	"having": true, "here": true, "however": true, "into": true, "itself": true,
	"just": true, "many": true, "more": true, "most": true, "much": true,
	"must": true, "myself": true, "never": true, "next": true, "once": true,
	"only": true, "other": true, "over": true, "please": true, "rather": true,
	"really": true, "same": true, "shall": true, "should": true, "since": true,
	"some": true, "something": true, "still": true, "such": true, "sure": true,
	"than": true, "that": true, "their": true, "them": true, "then": true,
	"there": true, "these": true, "they": true, "this": true, "those": true,
	"though": true, "through": true, "together": true, "under": true, "until": true,
	"upon": true, "used": true, "using": true, "very": true, "were": true,
	"what": true, "when": true, "where": true, "whether": true, "which": true,
	"while": true, "whom": true, "will": true, "with": true, "within": true,
	"without": true, "would": true, "your": true, "yours": true, "yourself": true,
}

// hintStatement is the query behind every hint. It is the search with two things
// added: a fact something later replaced is left out, because a withdrawn fact
// must never ride along in the prompt, and only the best few results are read,
// because the hint scores them itself afterwards.
const hintStatement = searchColumns + searchTables +
	` WHERE memory_search MATCH ? AND COALESCE(facts.superseded_by, '') = ''
	  ORDER BY bm25(memory_search), indexed.recorded DESC LIMIT ?`

// Hint returns at most three short lines for the end of the prompt, chosen by
// searching for the words of the step the agent is on, and nothing at all when
// the step offers no words worth searching for or when nothing holds enough of
// them. A fact the user withdrew never appears in a hint.
func (memory *Memory) Hint(ctx context.Context, query string) ([]string, error) {
	words := distinctWords(query, wordWorthHinting)
	if len(words) == 0 {
		return nil, nil
	}
	hits, err := memory.searchHits(ctx, hintStatement, []any{expressionOf(words), maxHintCandidates}, query)
	if err != nil {
		return nil, err
	}
	return hintLines(hits, words), nil
}

// hintLines keeps the results that hold enough of the step's own words, puts the
// ones holding the most first, and cuts each to a single short line.
func hintLines(hits []searchHit, words []string) []string {
	needed := hintWordsThatMustMatch
	if len(words) < needed {
		needed = len(words)
	}
	kept := []scoredHint{}
	for _, hit := range hits {
		line := oneLine(hit.asFact().Text)
		score := wordsHeldBy(line, words)
		if score >= needed {
			kept = append(kept, scoredHint{line: cutToRunes(line, maxHintRunes), score: score})
		}
	}
	slices.SortStableFunc(kept, func(first scoredHint, second scoredHint) int {
		return second.score - first.score
	})
	lines := []string{}
	for _, one := range kept {
		if len(lines) >= contract.MemoryHintLines {
			break
		}
		lines = append(lines, one.line)
	}
	return lines
}

// scoredHint is one candidate line together with how many of the step's own
// words it holds, which is what decides whether it is worth the prompt.
type scoredHint struct {
	line  string
	score int
}

// wordsHeldBy counts how many of the step's words a line holds, which is the
// hint's own measure of whether the line is about the same thing as the step.
// The search table's relevance measure cannot be used for this, because it
// scores against the size of the index rather than against the step.
func wordsHeldBy(line string, words []string) int {
	held := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(line), notPartOfAWord) {
		held[word] = true
	}
	found := 0
	for _, word := range words {
		if held[word] {
			found++
		}
	}
	return found
}

// wordWorthHinting says whether a word of a step is worth searching memory for:
// long enough to name something, and not one of the words every sentence holds.
func wordWorthHinting(word string) bool {
	return len([]rune(word)) >= minimumHintWordRunes && !hintStopWords[word]
}

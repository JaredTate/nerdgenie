package record

import "strings"

// The two numbers behind the size promise of the design: with a budget of a
// hundred rounds a record can hold at most a hundred result lines, so it stays
// small enough that nothing in it is ever squashed or summarized.
const (
	// TokensPerHundredWords is the ratio this package counts tokens with: a
	// hundred words of plain English are about a hundred and thirty tokens. It
	// is an estimate and it is deliberately the only one here, so that the size
	// of a record is always measured the same way. The working-context builder
	// of wave 2 keeps its own estimate for whole prompts.
	TokensPerHundredWords = 130
	// MaxRecordTokens is the size a record never passes, which is what lets the
	// design promise that the record fits in front of any model.
	MaxRecordTokens = 3000
	// MaxSummaryCharacters is the longest the one line a result keeps in the
	// record may be. It is the bound that makes the promise above hold at the
	// budget: a hundred rounds can write a hundred result lines and no more, and
	// each of those lines is this long at most. Nothing is lost by the cut,
	// because the whole text of every result is in the log.
	MaxSummaryCharacters = 70
)

// cutToOneLine keeps a result's summary to the one line a record holds for it,
// and says plainly that it was cut.
func cutToOneLine(summary string) string {
	letters := []rune(summary)
	if len(letters) <= MaxSummaryCharacters {
		return summary
	}
	return string(letters[:MaxSummaryCharacters-3]) + "..."
}

// EstimateTokens counts the tokens in a piece of text by splitting it on white
// space and applying the ratio above. It is an estimate, not a tokenizer: the
// point is a number that can be compared with MaxRecordTokens on every model.
func EstimateTokens(text string) int {
	return len(strings.Fields(text)) * TokensPerHundredWords / 100
}

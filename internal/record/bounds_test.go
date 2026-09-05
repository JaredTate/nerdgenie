package record

import "testing"

// TestEveryNamedNumberIsTheNumberItIsMeantToBe is finding 23 of the wave 6 gate
// review. Five of this package's six named numbers went red when they were
// changed, which is the standard the rest of the review is measured against;
// MaxCheckpoints did not, and could be set to seven with every test still
// passing. A number nothing pins is a number a careless edit moves.
//
// Each is written here as the literal it is meant to be, with the reason it is
// that number, so that changing one means changing this test and reading the
// reason first.
func TestEveryNamedNumberIsTheNumberItIsMeantToBe(t *testing.T) {
	for _, check := range []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxCheckpoints", MaxCheckpoints, 10000,
			"a task saves one checkpoint per round and a hundred rounds is its budget, so ten thousand is a log that has gone wrong rather than a record"},
		{"MaxRecordBytes", MaxRecordBytes, 1 << 20,
			"a megabyte is far more than the three thousand tokens a record is allowed, so anything larger is not a record at all"},
		{"MaxRecordLines", MaxRecordLines, 20000,
			"a record at its full size is a few hundred lines, so twenty thousand is the reader's guard rather than a working limit"},
		{"TokensPerHundredWords", TokensPerHundredWords, 130,
			"a hundred words of plain English are about a hundred and thirty tokens on every model this runs on"},
		{"MaxRecordTokens", MaxRecordTokens, 3000,
			"three thousand tokens is what lets the record sit in front of the smallest model a task may be picked up on"},
		{"MaxSummaryCharacters", MaxSummaryCharacters, 70,
			"seventy characters is one line, and a hundred of them are what a hundred-round budget can write"},
		{"MaxDoneLines", MaxDoneLines, 5,
			"five lines is what one sitting can prove, and an ask that needs more is a job with one task per line"},
		{"MaxPlanSteps", MaxPlanSteps, 10,
			"ten steps is what one sitting works through, the forty-step fixture plans its tweet in ten, and a longer plan is a job's task list in disguise"},
		{"MaxAskWordsForATask", MaxAskWordsForATask, 250,
			"a one-sitting ask is a sentence or a paragraph, a paragraph is about a hundred words, the forty-step fixture's ask is two sentences, and the game ask that was squeezed into one task runs to several hundred"},
	} {
		if check.is != check.want {
			t.Errorf("%s is %d and it is meant to be %d: %s", check.name, check.is, check.want, check.why)
		}
	}
}

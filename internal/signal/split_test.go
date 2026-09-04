package signal

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestSplitReplyKeepsAShortReplyWhole(t *testing.T) {
	reply := "Posted it.\n\nIt is 236 characters and mentions the date."
	pieces := SplitReply(reply)
	if len(pieces) != 1 {
		t.Fatalf("a short reply became %d messages, want one", len(pieces))
	}
	if pieces[0] != reply {
		t.Errorf("the one message is %q, want the reply unchanged", pieces[0])
	}
}

func TestSplitReplySendsNothingForAnEmptyReply(t *testing.T) {
	for _, reply := range []string{"", "   ", "\n\n\t\n"} {
		if pieces := SplitReply(reply); len(pieces) != 0 {
			t.Errorf("a reply of %q became %d messages, want none, because signal-cli refuses an empty message", reply, len(pieces))
		}
	}
}

func TestSplitReplyPacksParagraphsIntoTheFewestMessages(t *testing.T) {
	paragraph := strings.Repeat("word ", 160) + "end."
	reply := strings.Join([]string{paragraph, paragraph, paragraph}, "\n\n")

	pieces := SplitReply(reply)
	if len(pieces) != 2 {
		t.Fatalf("three paragraphs of about 805 characters became %d messages, want two, which is the fewest that fit", len(pieces))
	}
	for index, piece := range pieces {
		if signalLength(piece) > MessageLimit {
			t.Errorf("message %d is %d units long and the limit is %d", index+1, signalLength(piece), MessageLimit)
		}
	}
	if strings.Join(pieces, "\n\n") != reply {
		t.Errorf("the messages do not join back into the reply, so something was lost or changed at a paragraph break")
	}
}

func TestSplitReplyBreaksALongParagraphAtSentenceEnds(t *testing.T) {
	sentence := strings.Repeat("a", 96) + ". "
	reply := strings.Repeat(sentence, 30)

	pieces := SplitReply(reply)
	if len(pieces) < 2 {
		t.Fatalf("a paragraph of about 2940 characters became %d messages, want more than one", len(pieces))
	}
	for index, piece := range pieces {
		if signalLength(piece) > MessageLimit {
			t.Errorf("message %d is %d units long and the limit is %d", index+1, signalLength(piece), MessageLimit)
		}
		if !strings.HasSuffix(strings.TrimSpace(piece), ".") {
			t.Errorf("message %d ends %q, and a message is never cut in the middle of a sentence", index+1, lastFew(piece))
		}
	}
}

func TestSplitReplyBreaksALongSentenceAtWords(t *testing.T) {
	reply := strings.Repeat("word ", 600)

	pieces := SplitReply(reply)
	if len(pieces) < 2 {
		t.Fatalf("one sentence of 3000 characters became %d messages, want more than one", len(pieces))
	}
	for index, piece := range pieces {
		if signalLength(piece) > MessageLimit {
			t.Errorf("message %d is %d units long and the limit is %d", index+1, signalLength(piece), MessageLimit)
		}
		for _, word := range strings.Fields(piece) {
			if word != "word" {
				t.Errorf("message %d holds %q, and a message is never cut in the middle of a word while whole words fit", index+1, word)
			}
		}
	}
}

func TestSplitReplyCutsOneVeryLongWordOnACharacterBoundary(t *testing.T) {
	reply := strings.Repeat("é", 3000)

	pieces := SplitReply(reply)
	if len(pieces) < 2 {
		t.Fatalf("one word of 3000 characters became %d messages, want more than one", len(pieces))
	}
	for index, piece := range pieces {
		if signalLength(piece) > MessageLimit {
			t.Errorf("message %d is %d units long and the limit is %d", index+1, signalLength(piece), MessageLimit)
		}
		if !utf8.ValidString(piece) {
			t.Errorf("message %d is not valid text, so the cut fell inside a character", index+1)
		}
	}
}

func TestSplitReplyStopsAtTheMessageCapAndSaysSo(t *testing.T) {
	reply := strings.Repeat("word ", 6000)

	pieces := SplitReply(reply)
	if len(pieces) != MaxMessagesPerReply {
		t.Fatalf("a reply far past the cap became %d messages, want the cap of %d", len(pieces), MaxMessagesPerReply)
	}
	last := pieces[len(pieces)-1]
	if !strings.Contains(last, TruncationNote) {
		t.Errorf("the last message ends %q, want it to carry the note %q so the reader knows something was cut", lastFew(last), TruncationNote)
	}
	if signalLength(last) > MessageLimit {
		t.Errorf("the last message with its note is %d units long and the limit is %d", signalLength(last), MessageLimit)
	}
}

func TestSignalLengthCountsTheWayTheProtocolDoes(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"plain letters", "hello", 5},
		{"a letter with an accent", "é", 1},
		{"a character outside the basic range", "\U0001F600", 2},
		{"a mixture", "hi \U0001F600", 5},
		{"nothing", "", 0},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			if got := signalLength(oneCase.text); got != oneCase.want {
				t.Errorf("the length of %q is %d, want %d", oneCase.text, got, oneCase.want)
			}
		})
	}
}

func TestSplitReplyNeverCutsARedactionMarkerInHalf(t *testing.T) {
	// The reply is redacted before it is split, so the run a cut must not land
	// inside is the marker the redactor left behind. Half a marker in each of
	// two messages reads like the words it stands for.
	before := strings.Repeat("a", MessageLimit-len(contract.RedactedMarker)/2)
	reply := before + contract.RedactedMarker + " and that is the end of it."

	pieces := SplitReply(reply)
	if len(pieces) < 2 {
		t.Fatalf("the reply became %d messages, and this test needs one long enough to be split in two", len(pieces))
	}
	whole := 0
	for _, piece := range pieces {
		whole += strings.Count(piece, contract.RedactedMarker)
	}
	if whole != 1 {
		t.Errorf("%d whole markers survived the split, want one, so the cut landed inside %q: the first message ends %q and the second begins %q",
			whole, contract.RedactedMarker, lastFew(pieces[0]), firstFew(pieces[1]))
	}
}

// lastFew returns the tail of a message, for an error that has to show where a
// cut landed without printing two thousand characters.
func lastFew(piece string) string {
	letters := []rune(piece)
	if len(letters) <= 40 {
		return piece
	}
	return "..." + string(letters[len(letters)-40:])
}

// firstFew returns the head of a message, for the same reason lastFew returns
// its tail.
func firstFew(piece string) string {
	letters := []rune(piece)
	if len(letters) <= 40 {
		return piece
	}
	return string(letters[:40]) + "..."
}

// TestNoPieceOfASplitReplyIsEverEmpty is the fuzzer's finding: a reply made of
// carriage returns with a control character at its end came out as one piece
// holding nothing, which signal-cli refuses. Every piece holds a visible
// character, and a reply with none in it is not sent at all.
func TestNoPieceOfASplitReplyIsEverEmpty(t *testing.T) {
	reply := "\xff\xfe" + strings.Repeat("\r", 1400) + "\x1f"
	for at, piece := range SplitReply(reply) {
		if strings.TrimSpace(piece) == "" {
			t.Errorf("piece %d of the reply holds nothing", at+1)
		}
	}
	if pieces := SplitReply(strings.Repeat("\r", 3000)); len(pieces) != 0 {
		t.Errorf("a reply of nothing but carriage returns came out as %d pieces, want none", len(pieces))
	}
}

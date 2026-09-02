package repair_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/repair"
)

func TestABadCallIsAProblemUntilTwoParsesHaveAlreadyFailed(t *testing.T) {
	reply := writtenCallReply("frobnicate", "{}")

	cases := []struct {
		about        string
		failedParses int
		wantProblem  bool
	}{
		{about: "nothing has failed yet", failedParses: 0, wantProblem: true},
		{about: "one parse has failed", failedParses: 1, wantProblem: true},
		{about: "two parses have failed", failedParses: 2, wantProblem: false},
		{about: "three parses have failed", failedParses: 3, wantProblem: false},
		{about: "a count below zero counts as none", failedParses: -1, wantProblem: true},
	}

	for _, oneCase := range cases {
		t.Run(oneCase.about, func(t *testing.T) {
			result := repair.Find(reply, testSpecs(), oneCase.failedParses)
			if oneCase.wantProblem && result.Problem == "" {
				t.Fatalf("after %d failed parses the bad call produced no problem, want one", oneCase.failedParses)
			}
			if !oneCase.wantProblem && result.Problem != "" {
				t.Fatalf("after %d failed parses the bad call still produced the problem %q, want the text as the answer",
					oneCase.failedParses, result.Problem)
			}
			if len(result.Calls) != 0 {
				t.Errorf("a call that could not be repaired produced %d calls, want none", len(result.Calls))
			}
		})
	}
}

func TestAfterTwoFailedParsesTheTextThatLooksLikeACallIsTheAnswer(t *testing.T) {
	reply := writtenCallReply("frobnicate", "{}")

	result := repair.Find(reply, testSpecs(), repair.MaxFailedParses)

	if !strings.Contains(result.Text, "frobnicate") {
		t.Errorf("the answer is %q, want the whole reply including the text that looked like a call", result.Text)
	}
	if result.Note != "" {
		t.Errorf("giving up on the parse left the note %q, want none", result.Note)
	}
}

func TestAGoodCallStillRunsAfterTwoFailedParses(t *testing.T) {
	result := repair.Find(writtenCallReply("read", `{"path": "notes.md"}`), testSpecs(), repair.MaxFailedParses)

	if result.Problem != "" {
		t.Fatalf("a call that reads cleanly was refused with %q", result.Problem)
	}
	if len(result.Calls) != 1 || result.Calls[0].Name != "read" {
		t.Fatalf("a call that reads cleanly produced %+v, want one call to read", result.Calls)
	}
}

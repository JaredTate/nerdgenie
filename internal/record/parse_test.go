package record

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestParsesTheTaskExampleFromTheDesign proves the parser reads the task record
// in section 4 of the design back into exactly the types it came from.
func TestParsesTheTaskExampleFromTheDesign(t *testing.T) {
	parsed, err := Parse(readGolden(t, "task.txt"))
	if err != nil {
		t.Fatalf("the task example does not parse: %v", err)
	}
	if wanted := goldenTaskRecord(); !reflect.DeepEqual(parsed, wanted) {
		t.Errorf("the parsed task record is not the one the design wrote.\nwant %+v\ngot  %+v", wanted, parsed)
	}
}

// TestParsesTheJobExampleFromTheDesign proves the same for the job record.
func TestParsesTheJobExampleFromTheDesign(t *testing.T) {
	parsed, err := Parse(readGolden(t, "job.txt"))
	if err != nil {
		t.Fatalf("the job example does not parse: %v", err)
	}
	if wanted := goldenJobRecord(); !reflect.DeepEqual(parsed, wanted) {
		t.Errorf("the parsed job record is not the one the design wrote.\nwant %+v\ngot  %+v", wanted, parsed)
	}
}

// TestPrintsBackEveryByteItParsed is the round trip the brief asks for: read
// each golden record, write it out again, and compare the bytes.
func TestPrintsBackEveryByteItParsed(t *testing.T) {
	for _, name := range []string{"task.txt", "job.txt"} {
		original := readGolden(t, name)
		parsed, err := Parse(original)
		if err != nil {
			t.Fatalf("%s does not parse: %v", name, err)
		}
		if printed := Print(parsed); !bytes.Equal(printed, original) {
			t.Errorf("%s does not print back byte for byte.\n--- want ---\n%s\n--- got ---\n%s", name, original, printed)
		}
	}
}

// TestParsesWhatItPrints is the other half of the round trip: every record this
// package can hold prints to text that reads back as the same record.
func TestParsesWhatItPrints(t *testing.T) {
	for name, original := range recordsToRoundTrip() {
		printed := Print(original)
		parsed, err := Parse(printed)
		if err != nil {
			t.Errorf("the printed %s record does not parse: %v\n%s", name, err, printed)
			continue
		}
		if !reflect.DeepEqual(parsed, original) {
			t.Errorf("the %s record changed on the way through the text form.\nwant %+v\ngot  %+v\n%s", name, original, parsed, printed)
		}
	}
}

// recordsToRoundTrip is the set of records the round-trip test uses: the two from
// the design and the awkward shapes that are easy to get wrong.
func recordsToRoundTrip() map[string]contract.Record {
	bare := contract.Record{Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning}}

	folded := bare
	folded.Goal = contract.Goal{
		Ask: "first line\nsecond line\\here\rand a return",
		Why: `it has "quotes" and a -> arrow in it`,
	}

	replied := bare
	replied.Header.Status = contract.StatusWaiting
	replied.Goal = contract.Goal{DoneWhen: []contract.DoneLine{
		{Text: "the user says so", Done: true, UserReply: `yes, "that" is right`},
		{Text: "and this one is waiting"},
	}}

	emptyJob := contract.Record{Header: contract.Header{Kind: contract.RecordJob, ID: "9", Status: contract.StatusDone}}

	return map[string]contract.Record{
		"task from the design": goldenTaskRecord(),
		"job from the design":  goldenJobRecord(),
		"bare task":            bare,
		"folded text":          folded,
		"a reply as the proof": replied,
		"bare job":             emptyJob,
	}
}

// TestTheOneLetterLabelsAgreeWithTheContract holds this package's labels to the
// identifiers the contract writes, because the two must never drift apart.
func TestTheOneLetterLabelsAgreeWithTheContract(t *testing.T) {
	ours := []string{correctionLabel + "1", decisionLabel + "7", failureLabel + "12"}
	theirs := []string{contract.CorrectionID(1), contract.DecisionID(7), contract.FailureID(12)}
	for at, mine := range ours {
		if mine != theirs[at] {
			t.Errorf("this package writes %q where the contract writes %q", mine, theirs[at])
		}
	}
}

// readGolden reads one of the two records the design wrote, from testdata.
func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("cannot read the golden record %s: %v", name, err)
	}
	return content
}

package replay_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAsTestWritesTheTestThatIsCheckedIn is how "a test that compiles and
// passes" is proved: the two files the generator writes are the two files that
// sit under test/replays in this repository, and those are compiled and run by
// every "make test". If the generator changes, this test says so instead of
// letting the checked-in copy quietly become something else.
func TestAsTestWritesTheTestThatIsCheckedIn(t *testing.T) {
	generated := generateFromACleanRun(t)

	root := filepath.Join("..", "..")
	for _, one := range []struct {
		path    string
		content []byte
	}{
		{filepath.Join(root, generated.TestPath), generated.TestFile},
		{filepath.Join(root, generated.FixturePath), generated.FixtureFile},
	} {
		if os.Getenv(testkit.UpdateGoldenFilesVariable) == "1" {
			if err := testkit.WriteGolden(one.path, one.content); err != nil {
				t.Fatalf("cannot write %s: %v", one.path, err)
			}
			continue
		}
		same, err := testkit.GoldenMatches(one.path, one.content)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !same {
			t.Errorf("%s is not what the generator writes today.\nRead the difference, and run the tests with %s=1 only when the new file is right.",
				one.path, testkit.UpdateGoldenFilesVariable)
		}
	}
}

func TestAsTestNamesBothFilesAfterTheTask(t *testing.T) {
	generated := generateFromACleanRun(t)

	if generated.TestPath != "test/replays/task_1_test.go" {
		t.Errorf("the generated test is %q and it belongs under test/replays", generated.TestPath)
	}
	if generated.FixturePath != "test/replays/testdata/task-1.json" {
		t.Errorf("the generated recording is %q and it belongs beside the test", generated.FixturePath)
	}
	if !strings.Contains(string(generated.TestFile), "func TestTask1ReplaysTheSameWay") {
		t.Errorf("the generated test holds no test function:\n%s", generated.TestFile)
	}
}

func TestAsTestRefusesATaskNumberThatIsNotAName(t *testing.T) {
	_, err := replay.AsTest(replay.Recording{TaskID: "../secrets"})
	if err == nil {
		t.Fatal("a task whose number is a path gave no error")
	}
	if !strings.Contains(err.Error(), "../secrets") {
		t.Errorf("the error is %q and it must name what it refused", err)
	}
}

func TestARecordingReadsBackExactlyAsItWasWritten(t *testing.T) {
	made := runScriptWithACorrection(t)
	recording, err := replay.Read(context.Background(), made.store, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot read the recording: %v", err)
	}

	written, err := recording.Encode()
	if err != nil {
		t.Fatalf("cannot write the recording out: %v", err)
	}
	read, err := replay.Decode(written)
	if err != nil {
		t.Fatalf("cannot read the recording back: %v", err)
	}

	if read.Ask != recording.Ask || read.Answer != recording.Answer {
		t.Errorf("the recording came back with the ask %q and the answer %q", read.Ask, read.Answer)
	}
	if len(read.Rounds) != len(recording.Rounds) {
		t.Fatalf("the recording came back with %d rounds and it had %d", len(read.Rounds), len(recording.Rounds))
	}
	if read.FinalText != recording.FinalText {
		t.Errorf("the recorded record came back as\n%s\nand it was\n%s", read.FinalText, recording.FinalText)
	}
	if read.Final.Header.Status != recording.Final.Header.Status {
		t.Errorf("the recorded record came back %q and it was %q", read.Final.Header.Status, recording.Final.Header.Status)
	}
}

func TestDecodeRefusesWhatIsNotARecording(t *testing.T) {
	refused := map[string]string{
		"text that is not JSON at all": "this is not a recording",
		"a record that will not parse": `{"taskId":"1","finalText":"not a record at all"}`,
	}
	for what, written := range refused {
		t.Run(what, func(t *testing.T) {
			if _, err := replay.Decode([]byte(written)); err == nil {
				t.Fatalf("decoding %s gave no error", what)
			}
		})
	}
}

// generateFromACleanRun records the two-round task and turns it into the pair
// of files the "--as-test" flag writes.
func generateFromACleanRun(t *testing.T) replay.GeneratedTest {
	t.Helper()
	made := runScript(t, twoRoundScript())
	recording, err := replay.Read(context.Background(), made.store, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot read the recording of task %s: %v", made.outcome.TaskID, err)
	}
	generated, err := replay.AsTest(recording)
	if err != nil {
		t.Fatalf("cannot turn the recording of task %s into a test: %v", made.outcome.TaskID, err)
	}
	return generated
}

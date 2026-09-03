package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// TestsFolder is where the tests written by "coeus replay <task> --as-test"
// live, as a path from the top of the repository. They are ordinary Go tests,
// so "go test ./..." runs them like any other.
const TestsFolder = "test/replays"

// GeneratedTest is the pair of files one "--as-test" writes: the Go test, and
// the recording it replays, which sits in the testdata folder beside it.
type GeneratedTest struct {
	// TestPath is where the test goes, as a path from the top of the repository.
	TestPath string
	// TestFile is the Go source of the test.
	TestFile []byte
	// FixturePath is where the recording goes, beside the test.
	FixturePath string
	// FixtureFile is the recording, written out.
	FixtureFile []byte
}

// AsTest turns one recording into a Go test that replays it. The test carries
// no part of the machine it was written on: the recording travels beside it and
// everything else comes out of internal/testkit, so the test runs anywhere the
// repository does.
func AsTest(recording Recording) (GeneratedTest, error) {
	name, err := nameOfTask(recording.TaskID)
	if err != nil {
		return GeneratedTest{}, err
	}
	written, err := recording.Encode()
	if err != nil {
		return GeneratedTest{}, err
	}
	fixture := path.Join(TestsFolder, "testdata", "task-"+name+".json")
	return GeneratedTest{
		TestPath:    path.Join(TestsFolder, "task_"+name+"_test.go"),
		TestFile:    []byte(testSourceFor(name, path.Join("testdata", "task-"+name+".json"))),
		FixturePath: fixture,
		FixtureFile: written,
	}, nil
}

// WriteInto writes both files under the folder given, which is the top of the
// repository, making the folders they sit in when they are not there.
func (generated GeneratedTest) WriteInto(root string) error {
	for _, one := range []struct {
		where string
		what  []byte
	}{
		{generated.TestPath, generated.TestFile},
		{generated.FixturePath, generated.FixtureFile},
	} {
		where := filepath.Join(root, filepath.FromSlash(one.where))
		if err := os.MkdirAll(filepath.Dir(where), contract.HomeFolderMode); err != nil {
			return fmt.Errorf("cannot make the folder for %s: %w", where, err)
		}
		if err := os.WriteFile(where, one.what, contract.DataFileMode); err != nil {
			return fmt.Errorf("cannot write %s: %w", where, err)
		}
	}
	return nil
}

// nameOfTask is the task's number as it may appear in a file name and in a Go
// function name. A task is numbered, so anything else is refused rather than
// turned into a path nobody meant to write.
func nameOfTask(taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("a recording with no task number cannot be written as a test, so replay a task that has one")
	}
	for _, letter := range taskID {
		if letter < '0' || letter > '9' {
			return "", fmt.Errorf("the task number %q is not a number, and a generated test is named after one, so replay a numbered task", taskID)
		}
	}
	return taskID, nil
}

// Encode writes a recording out as the file that travels beside a generated
// test. The record itself travels as the text a record prints as, because that
// is the one form internal/record promises reads back the same.
func (recording Recording) Encode() ([]byte, error) {
	recording.FinalText = string(record.Print(recording.Final))
	written, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cannot write the recording of task %s out: %w", recording.TaskID, err)
	}
	return append(written, '\n'), nil
}

// Decode reads a recording back out of such a file.
func Decode(raw []byte) (Recording, error) {
	recording := Recording{}
	if err := json.Unmarshal(raw, &recording); err != nil {
		return Recording{}, fmt.Errorf("this is not a recording written by coeus replay: %w", err)
	}
	held, err := record.Parse([]byte(recording.FinalText))
	if err != nil {
		return Recording{}, fmt.Errorf("the recording of task %s carries a record that will not read back: %w", recording.TaskID, err)
	}
	recording.Final = held
	return recording, nil
}

// testSourceFor writes the Go source of one generated test. It is written out
// in full rather than through a template, because a reader who opens the
// generated file should see a test they could have written themselves.
func testSourceFor(name string, fixture string) string {
	return strings.ReplaceAll(strings.ReplaceAll(theGeneratedTest, "<name>", name), "<fixture>", fixture)
}

// theGeneratedTest is the shape of every generated test, with the task's number
// and the path of its recording put in.
const theGeneratedTest = `// This file was written by "coeus replay <name> --as-test". It replays the
// recording that sits beside it against the code as it stands now, and fails
// when the run no longer ends where it ended when it was recorded.
//
// The rulebook here allows every call, because a test that has to run on any
// machine cannot read this one's config.toml. A recorded task that had a call
// refused needs that rule written in below by hand.

package replays

import (
	"context"
	"os"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/replay"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTask<name>ReplaysTheSameWay(t *testing.T) {
	held, err := os.ReadFile("<fixture>")
	if err != nil {
		t.Fatalf("cannot read the recording beside this test: %v", err)
	}
	recording, err := replay.Decode(held)
	if err != nil {
		t.Fatalf("cannot read the recording beside this test: %v", err)
	}
	built, err := workingcontext.New(workingcontext.Options{
		Home:            testkit.NewTempHome(t),
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
	})
	if err != nil {
		t.Fatalf("cannot build the working context this replay needs: %v", err)
	}
	result, err := replay.RunRecording(context.Background(), replay.Options{
		Into:       testkit.NewFakeStore(),
		Context:    loop.TheWorkingContext(built),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)),
	}, recording)
	if err != nil {
		t.Fatalf("cannot replay task <name>: %v", err)
	}
	if !result.Passed {
		t.Fatalf("task <name> no longer replays the way it was recorded:\n%s", result.Report)
	}
}
`

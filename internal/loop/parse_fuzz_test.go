package loop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// FuzzReadRecordUpdate throws any arguments at the reader of a task call. It
// must never panic, and it must either return an update the record could take or
// say what to write instead.
func FuzzReadRecordUpdate(f *testing.F) {
	f.Add(`{"why":"because"}`)
	f.Add(`{"doneWhen":["a",{"text":"b","done":true,"resultId":"r1"}]}`)
	f.Add(`{"tasks":[{"taskId":"t3","text":"post","dueAt":"today"}]}`)
	f.Add(`{"decision":{"text":"go","reason":"why"},"failure":{"text":"no","cause":"why"}}`)
	f.Add(`[]`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, arguments string) {
		update, err := readRecordUpdate(json.RawMessage(arguments))
		if err != nil {
			if !strings.Contains(err.Error(), "task tool") && !strings.Contains(err.Error(), "record") {
				t.Fatalf("the refusal reads %q, and it must say what to write instead", err)
			}
			return
		}
		if nothingWritten(update) {
			t.Fatalf("the arguments %q were read as a change that changes nothing", arguments)
		}
	})
}

// FuzzWhatTheHarnessReadsFromText throws any text at the small readers that pick
// facts out of what a tool returned and what a done line named. None of them may
// panic, and every one of them is bounded.
func FuzzWhatTheHarnessReadsFromText(f *testing.F) {
	f.Add("the page is a login page", "run `go test ./...` and open /tmp/one.md")
	f.Add("exit 0\n", "the file ~/notes/a.md is written")
	f.Add("", "")
	f.Add("`````", "//////")

	f.Fuzz(func(t *testing.T, result string, doneLine string) {
		if _, found := exitCodeIn(result); found && result == "" {
			t.Fatal("an exit code was read out of no text at all")
		}
		if len(commandsIn(doneLine)) > MaxCommandsCheckedPerLine {
			t.Fatalf("the line %q was read as more commands than the cap allows", doneLine)
		}
		if len(pathsIn(doneLine)) > MaxPathsCheckedPerLine {
			t.Fatalf("the line %q was read as more paths than the cap allows", doneLine)
		}
		if line := cutToALine(result); len([]rune(line)) > MaxSituationLineLetters {
			t.Fatalf("a situation line came out %d letters long, and the cap is %d", len([]rune(line)), MaxSituationLineLetters)
		}
		fingerprintOf(contract.ToolCall{Name: doneLine, Input: json.RawMessage(result)})
		summaryOfResult(contract.ToolTask, result, false)
	})
}

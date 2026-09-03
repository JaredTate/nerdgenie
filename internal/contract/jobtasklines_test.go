package contract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheJobStatusFieldsAreNamed pins the four fields that carry the job a
// screen is watching, so that the program and the screen spell them the same.
func TestTheJobStatusFieldsAreNamed(t *testing.T) {
	for field, want := range map[string]string{
		contract.StatusFieldJob:      "job",
		contract.StatusFieldJobAsk:   "jobAsk",
		contract.StatusFieldJobTask:  "jobTask",
		contract.StatusFieldJobTasks: "jobTasks",
	} {
		if field != want {
			t.Errorf("status field is %q, want %q", field, want)
		}
	}
}

// theCampaignTasks is the task list of the job in section 4 of COEUS.md, with
// the check marks and the dates the record carries.
func theCampaignTasks() []contract.JobTask {
	return []contract.JobTask{
		{TaskID: "t17", Text: "post the anniversary tweet", Done: true, ReportID: "j4.1"},
		{TaskID: "t19", Text: "draft the blog piece", Done: true, ReportID: "j4.2"},
		{TaskID: "t31", Text: "post for day three", DueAt: "today at 14:00"},
		{TaskID: "t40", Text: "write the summary\nfor the user, after the last post"},
	}
}

func TestAJobsTaskListIsWrittenOneTaskPerLineWithItsMark(t *testing.T) {
	written := contract.JobTaskLines(theCampaignTasks())

	want := "[x] t17 post the anniversary tweet\n" +
		"[x] t19 draft the blog piece\n" +
		"[ ] t31 post for day three\n" +
		"[ ] t40 write the summary for the user, after the last post"
	if written != want {
		t.Errorf("the task list is written as:\n%s\nwant:\n%s", written, want)
	}
}

func TestAJobsTaskListReadsBackWhatWasWritten(t *testing.T) {
	read := contract.ParseJobTaskLines(contract.JobTaskLines(theCampaignTasks()))

	if len(read) != 4 {
		t.Fatalf("read %d tasks back, want 4: %+v", len(read), read)
	}
	for at, want := range []contract.JobTask{
		{TaskID: "t17", Text: "post the anniversary tweet", Done: true},
		{TaskID: "t19", Text: "draft the blog piece", Done: true},
		{TaskID: "t31", Text: "post for day three"},
		{TaskID: "t40", Text: "write the summary for the user, after the last post"},
	} {
		if read[at] != want {
			t.Errorf("task %d reads back as %+v, want %+v", at+1, read[at], want)
		}
	}
}

func TestALineInAnotherShapeIsReadAsATaskNotDone(t *testing.T) {
	read := contract.ParseJobTaskLines("\n  post the tweet  \n[x]\nt3\n")

	want := []contract.JobTask{
		{Text: "post the tweet"},
		{Text: "[x]"},
		{TaskID: "t3"},
	}
	if len(read) != len(want) {
		t.Fatalf("read %d tasks, want %d: %+v", len(read), len(want), read)
	}
	for at := range want {
		if read[at] != want[at] {
			t.Errorf("line %d reads back as %+v, want %+v", at+1, read[at], want[at])
		}
	}
	if len(contract.ParseJobTaskLines("")) != 0 {
		t.Error("an empty field reads back as tasks")
	}
}

func FuzzParseJobTaskLines(f *testing.F) {
	f.Add(contract.JobTaskLines(theCampaignTasks()))
	f.Add("[ ] \n[x] \n")
	f.Add("[x]t1 one\n[ ]  t2  two")
	f.Fuzz(func(t *testing.T, text string) {
		read := contract.ParseJobTaskLines(text)
		if len(read) > strings.Count(text, "\n")+1 {
			t.Errorf("read %d tasks out of %d lines", len(read), strings.Count(text, "\n")+1)
		}
		// Writing what was read and reading it again lands on the same tasks,
		// because a task's text is one line and its mark is one of two.
		again := contract.ParseJobTaskLines(contract.JobTaskLines(read))
		if len(again) != len(read) {
			t.Fatalf("read %d tasks, wrote them, and read %d back", len(read), len(again))
		}
		for at := range read {
			if again[at] != read[at] {
				t.Errorf("task %d read back as %+v after being written, was %+v", at+1, again[at], read[at])
			}
		}
	})
}

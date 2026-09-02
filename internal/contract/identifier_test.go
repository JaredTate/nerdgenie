package contract_test

import (
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestIdentifiersAreFormattedTheWayTheDesignShowsThem(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"a task result", contract.ResultID(7), "r7"},
		{"a job report", contract.ReportID("4", 2), "j4.2"},
		{"a task inside a job", contract.TaskID(31), "t31"},
		{"a correction", contract.CorrectionID(1), "C1"},
		{"a decision", contract.DecisionID(1), "D1"},
		{"a failure", contract.FailureID(1), "F1"},
		{"a page element", contract.ElementRef(12), "e12"},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Errorf("the id of %s is %q, want %q", test.name, test.got, test.want)
		}
	}
}

func TestParseResultIDReadsWhatResultIDWrote(t *testing.T) {
	tests := []struct {
		id     string
		number int
		valid  bool
	}{
		{"r7", 7, true},
		{"r1", 1, true},
		{"r100", 100, true},
		{"r0", 0, false},
		{"r", 0, false},
		{"r-1", 0, false},
		{"7", 0, false},
		{"j4.2", 0, false},
		{"rseven", 0, false},
		{"", 0, false},
		{"r 7", 0, false},
		{"r007", 0, false},
	}
	for _, test := range tests {
		number, valid := contract.ParseResultID(test.id)
		if valid != test.valid {
			t.Errorf("ParseResultID(%q) said valid is %v, want %v", test.id, valid, test.valid)
			continue
		}
		if valid && number != test.number {
			t.Errorf("ParseResultID(%q) read %d, want %d", test.id, number, test.number)
		}
	}
}

func TestParseReportIDReadsTheJobAndTheNumber(t *testing.T) {
	tests := []struct {
		id     string
		job    string
		number int
		valid  bool
	}{
		{"j4.2", "4", 2, true},
		{"j17.1", "17", 1, true},
		{"j4", "", 0, false},
		{"j4.", "", 0, false},
		{"j.2", "", 0, false},
		{"j4.0", "", 0, false},
		{"j0.2", "", 0, false},
		{"r7", "", 0, false},
		{"", "", 0, false},
		{"j4.2.3", "", 0, false},
	}
	for _, test := range tests {
		job, number, valid := contract.ParseReportID(test.id)
		if valid != test.valid {
			t.Errorf("ParseReportID(%q) said valid is %v, want %v", test.id, valid, test.valid)
			continue
		}
		if valid && (job != test.job || number != test.number) {
			t.Errorf("ParseReportID(%q) read job %q number %d, want job %q number %d", test.id, job, number, test.job, test.number)
		}
	}
}

func TestParseTaskIDReadsWhatTaskIDWrote(t *testing.T) {
	tests := []struct {
		id     string
		number int
		valid  bool
	}{
		{"t31", 31, true},
		{"t1", 1, true},
		{"t0", 0, false},
		{"t", 0, false},
		{"31", 0, false},
		{"", 0, false},
	}
	for _, test := range tests {
		number, valid := contract.ParseTaskID(test.id)
		if valid != test.valid {
			t.Errorf("ParseTaskID(%q) said valid is %v, want %v", test.id, valid, test.valid)
			continue
		}
		if valid && number != test.number {
			t.Errorf("ParseTaskID(%q) read %d, want %d", test.id, number, test.number)
		}
	}
}

func FuzzParseResultID(f *testing.F) {
	for _, seed := range []string{"r7", "j4.2", "", "r", "r-0", "r99999999999999999999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) {
		number, valid := contract.ParseResultID(id)
		if valid && contract.ResultID(number) != id {
			t.Fatalf("ParseResultID(%q) read %d, but ResultID(%d) writes %q", id, number, number, contract.ResultID(number))
		}
	})
}

func FuzzParseReportID(f *testing.F) {
	for _, seed := range []string{"j4.2", "j.", "j..", "", "j1.1"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) {
		job, number, valid := contract.ParseReportID(id)
		if valid && contract.ReportID(job, number) != id {
			t.Fatalf("ParseReportID(%q) read job %q number %d, but ReportID writes %q", id, job, number, contract.ReportID(job, number))
		}
	})
}

func TestTheLogKeyKeepsATaskAndAJobWithTheSameNumberApart(t *testing.T) {
	task := contract.RecordLogKey(contract.RecordTask, "17")
	job := contract.RecordLogKey(contract.RecordJob, "17")
	if task == job {
		t.Fatalf("task 17 and job 17 share the log key %q, and their events would mix", task)
	}
	if task != "17" {
		t.Errorf("a task's log key is %q, want its own number", task)
	}
	if job != "j17" {
		t.Errorf("a job's log key is %q, want j17", job)
	}
	if _, _, valid := contract.ParseReportID(job); valid {
		t.Errorf("the job's log key %q reads as a report id, and the two must not collide", job)
	}
}

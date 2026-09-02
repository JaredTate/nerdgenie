package config

import (
	"fmt"
	"strings"
)

// Result says how one of the doctor's checks came out. There are three, and the
// line between the last two is the line between a thing that is switched off and
// a thing that is broken: a machine with no browser installed is working
// properly with the browser tools switched off, while a vault key anyone on the
// machine can read is broken and has to be fixed.
type Result string

const (
	// Fine means the check found what it was looking for.
	Fine Result = "ok"
	// Warning means something Coeus can work without is missing, and says what
	// is switched off because of it.
	Warning Result = "warning"
	// Trouble means something is wrong and has to be put right.
	Trouble Result = "problem"
)

// Finding is one line of the doctor's report: one thing looked at, how it came
// out, and what was found.
type Finding struct {
	// What names the thing looked at, such as "config.toml" or "bwrap".
	What string
	// Result says how it came out.
	Result Result
	// Detail says what was found, and what to do when it is not right.
	Detail string
}

// Report is everything the doctor looked at, in the order it looked, with the
// home folder it looked in.
type Report struct {
	// Root is the home folder the report is about.
	Root string
	// Findings is one line per check.
	Findings []Finding
}

// Verdict is the worst of the findings, which is what a caller acts on: a
// problem anywhere makes the whole report a problem.
func (report Report) Verdict() Result {
	verdict := Fine
	for _, finding := range report.Findings {
		if finding.Result == Trouble {
			return Trouble
		}
		if finding.Result == Warning {
			verdict = Warning
		}
	}
	return verdict
}

// String prints the report as plain text a person can read: a heading naming the
// home folder, one line per check with its result and its detail, and a closing
// sentence saying what the verdict means.
func (report Report) String() string {
	widest := 0
	for _, finding := range report.Findings {
		if len(finding.What) > widest {
			widest = len(finding.What)
		}
	}

	written := &strings.Builder{}
	fmt.Fprintf(written, "coeus doctor: %s\n\n", report.Root)
	for _, finding := range report.Findings {
		fmt.Fprintf(written, "  %-7s  %-*s  %s\n", finding.Result, widest, finding.What, finding.Detail)
	}
	fmt.Fprintf(written, "\n%s\n", report.closingSentence())
	return written.String()
}

// closingSentence says what the verdict means and how many things it is about.
func (report Report) closingSentence() string {
	warnings, problems := 0, 0
	for _, finding := range report.Findings {
		switch finding.Result {
		case Warning:
			warnings++
		case Trouble:
			problems++
		case Fine:
		}
	}
	switch report.Verdict() {
	case Trouble:
		return fmt.Sprintf("problem: %d things need fixing before Coeus will work properly, and %d more are switched off",
			problems, warnings)
	case Warning:
		return fmt.Sprintf("warning: Coeus will run, with %d things switched off", warnings)
	default:
		return "ok: everything Coeus needs is here"
	}
}

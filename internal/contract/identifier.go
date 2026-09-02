package contract

import (
	"strconv"
	"strings"
)

// The prefixes the design uses for the identifiers in a record. They are one
// character each, because every one of them is read by a person and written by a
// model on every turn.
const (
	resultPrefix     = "r"
	reportPrefix     = "j"
	taskPrefix       = "t"
	correctionPrefix = "C"
	decisionPrefix   = "D"
	failurePrefix    = "F"
	elementPrefix    = "e"
)

// ResultID returns the identifier of a task result, such as "r7".
func ResultID(number int) string {
	return resultPrefix + strconv.Itoa(number)
}

// ParseResultID reads a task result identifier such as "r7" and returns its
// number. It returns false for anything that ResultID would not have written,
// including a leading zero and a number below one.
func ParseResultID(id string) (int, bool) {
	return parseNumberedID(id, resultPrefix)
}

// ReportID returns the identifier of one finished task's report inside a job,
// such as "j4.2" for the second report of job 4.
func ReportID(jobID string, number int) string {
	return reportPrefix + jobID + "." + strconv.Itoa(number)
}

// ParseReportID reads a job report identifier such as "j4.2" and returns the job
// and the report number. It returns false for anything ReportID would not have
// written.
func ParseReportID(id string) (string, int, bool) {
	rest, found := strings.CutPrefix(id, reportPrefix)
	if !found {
		return "", 0, false
	}
	jobID, numberText, split := strings.Cut(rest, ".")
	if !split || strings.Contains(numberText, ".") {
		return "", 0, false
	}
	if !positiveNumber(jobID) {
		return "", 0, false
	}
	number, valid := positiveNumberValue(numberText)
	if !valid {
		return "", 0, false
	}
	return jobID, number, true
}

// TaskID returns the identifier of one task inside a job, such as "t31".
func TaskID(number int) string {
	return taskPrefix + strconv.Itoa(number)
}

// ParseTaskID reads a task identifier such as "t31" and returns its number.
func ParseTaskID(id string) (int, bool) {
	return parseNumberedID(id, taskPrefix)
}

// CorrectionID returns the identifier of a correction, such as "C1".
func CorrectionID(number int) string {
	return correctionPrefix + strconv.Itoa(number)
}

// DecisionID returns the identifier of a decision, such as "D1".
func DecisionID(number int) string {
	return decisionPrefix + strconv.Itoa(number)
}

// FailureID returns the identifier of a failure, such as "F1".
func FailureID(number int) string {
	return failurePrefix + strconv.Itoa(number)
}

// RecordLogKey returns the id a record's events are logged under: a task's own
// number, and "j" followed by the number for a job, because task 17 and job 17
// are different records and their events must never mix in the log.
func RecordLogKey(kind RecordKind, id string) string {
	if kind == RecordJob {
		return reportPrefix + id
	}
	return id
}

// ElementRef returns the short label of one element on a web page, such as
// "e12".
func ElementRef(number int) string {
	return elementPrefix + strconv.Itoa(number)
}

// parseNumberedID reads a prefix followed by a positive number with no leading
// zero, which is the one shape every identifier in this file shares.
func parseNumberedID(id string, prefix string) (int, bool) {
	rest, found := strings.CutPrefix(id, prefix)
	if !found {
		return 0, false
	}
	return positiveNumberValue(rest)
}

// positiveNumberValue reads a decimal number of one or more digits that is at
// least one and carries no leading zero and no sign.
func positiveNumberValue(text string) (int, bool) {
	if !positiveNumber(text) {
		return 0, false
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return number, true
}

// positiveNumber says whether the text is a decimal number of at least one, with
// no sign, no space, and no leading zero.
func positiveNumber(text string) bool {
	if text == "" || text[0] == '0' {
		return false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

package job

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// dayNames are the days of the week as a cron expression numbers them, with
// Sunday at both ends, because crontab writes Sunday as either nought or seven.
var dayNames = map[string]string{
	"0": "Sunday", "7": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
	"4": "Thursday", "5": "Friday", "6": "Saturday",
	"sun": "Sunday", "mon": "Monday", "tue": "Tuesday", "wed": "Wednesday",
	"thu": "Thursday", "fri": "Friday", "sat": "Saturday",
}

// descriptionsInWords are the shorthand schedules cron understands, each said the
// way a person says it.
var descriptionsInWords = map[string]string{
	"@yearly": "once a year on 1 January at midnight", "@annually": "once a year on 1 January at midnight",
	"@monthly": "every month on the 1st at midnight", "@weekly": "every Sunday at midnight",
	"@daily": "every day at midnight", "@midnight": "every day at midnight", "@hourly": "every hour",
}

// cronInWords says a cron expression the way a person says it. The shapes people
// actually write are put into words, and anything else is printed as the
// expression it is, because a wrong sentence is worse than a true one nobody
// reads out loud.
func cronInWords(expression string) string {
	trimmed := strings.TrimSpace(expression)
	if said, known := descriptionsInWords[strings.ToLower(trimmed)]; known {
		return said
	}
	if rest, isEvery := strings.CutPrefix(strings.ToLower(trimmed), "@every "); isEvery {
		return "every " + strings.TrimSpace(rest)
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 5 {
		return asTheExpressionItIs(expression)
	}
	minute, hour, dayOfMonth, month, dayOfWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

	if said, known := everySoOften(minute, hour, dayOfMonth, month, dayOfWeek); known {
		return said
	}
	atTime, known := timeOfDayOfFields(minute, hour)
	if !known || month != "*" {
		return asTheExpressionItIs(expression)
	}
	if dayOfMonth != "*" && dayOfWeek == "*" {
		if day, isNumber := wholeNumber(dayOfMonth, 1, 31); isNumber {
			return fmt.Sprintf("every month on the %s at %s", ordinal(day), atTime)
		}
		return asTheExpressionItIs(expression)
	}
	if dayOfMonth != "*" {
		return asTheExpressionItIs(expression)
	}
	days, known := daysInWords(dayOfWeek)
	if !known {
		return asTheExpressionItIs(expression)
	}
	return days + " at " + atTime
}

// everySoOften says the two shapes that repeat inside the day rather than
// happening at one time of it: every so many minutes, and every so many hours.
func everySoOften(minute string, hour string, dayOfMonth string, month string, dayOfWeek string) (string, bool) {
	if dayOfMonth != "*" || month != "*" || dayOfWeek != "*" {
		return "", false
	}
	if step, isStep := everyStep(minute, 59); isStep && hour == "*" {
		return "every " + intervalInWords(time.Duration(step)*time.Minute), true
	}
	step, isStep := everyStep(hour, 23)
	if !isStep {
		return "", false
	}
	if _, isNumber := wholeNumber(minute, 0, 59); !isNumber {
		return "", false
	}
	return "every " + intervalInWords(time.Duration(step)*time.Hour), true
}

// timeOfDayOfFields reads the minute and hour fields as one time of day, and says
// no when either is anything but a plain number.
func timeOfDayOfFields(minute string, hour string) (string, bool) {
	atMinute, isMinute := wholeNumber(minute, 0, 59)
	atHour, isHour := wholeNumber(hour, 0, 23)
	if !isMinute || !isHour {
		return "", false
	}
	return timeOfDayInWords(atHour, atMinute), true
}

// daysInWords says a cron expression's day-of-week field the way a person says
// it: every day, every weekday, or the days themselves by name.
func daysInWords(dayOfWeek string) (string, bool) {
	folded := strings.ToLower(strings.TrimSpace(dayOfWeek))
	switch folded {
	case "*", "?":
		return "every day", true
	case "1-5", "mon-fri":
		return "every weekday", true
	case "0,6", "6,0", "6-7", "sat,sun", "sun,sat":
		return "every Saturday and Sunday", true
	}
	named := []string{}
	for _, part := range strings.Split(folded, ",") {
		name, known := dayNames[strings.TrimSpace(part)]
		if !known {
			return "", false
		}
		named = append(named, name)
	}
	if len(named) == 0 || len(named) > 7 {
		return "", false
	}
	return "every " + listInWords(named), true
}

// everyStep reads a field written as a step, such as "*/15", and returns the
// step when it is one this printer can say out loud.
func everyStep(field string, highest int) (int, bool) {
	rest, isStep := strings.CutPrefix(field, "*/")
	if !isStep {
		return 0, false
	}
	step, isNumber := wholeNumber(rest, 1, highest)
	if !isNumber {
		return 0, false
	}
	return step, true
}

// wholeNumber reads a field as a plain number inside the range given, and says no
// to everything else, including a list, a range, and a name.
func wholeNumber(field string, lowest int, highest int) (int, bool) {
	number, err := strconv.Atoi(field)
	if err != nil || number < lowest || number > highest {
		return 0, false
	}
	return number, true
}

// ordinal writes a day of the month the way a person writes it, such as "3rd".
func ordinal(day int) string {
	suffix := "th"
	switch {
	case day%100 >= 11 && day%100 <= 13:
		suffix = "th"
	case day%10 == 1:
		suffix = "st"
	case day%10 == 2:
		suffix = "nd"
	case day%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(day) + suffix
}

// asTheExpressionItIs is what an expression too unusual to put into words is
// printed as. It says less than a sentence and it is never wrong.
func asTheExpressionItIs(expression string) string {
	return "on the cron schedule " + strconv.Quote(strings.TrimSpace(expression))
}

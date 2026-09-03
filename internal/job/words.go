package job

import (
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// InPlainWords says a schedule the way a person says it: "every weekday at 7 in
// the morning", "every 2 hours", "once on 2026-10-01 at 09:00". It is what
// "/cron" prints, and it never fails: a schedule too unusual to put into words
// is printed as the cron expression it is, which is still true.
func InPlainWords(schedule contract.Schedule) string {
	said := ""
	switch schedule.Kind {
	case contract.ScheduleAt:
		said = "once on " + schedule.At.Format("2006-01-02 at 15:04")
	case contract.ScheduleEvery:
		said = "every " + intervalInWords(schedule.Every)
	case contract.ScheduleCron:
		said = cronInWords(schedule.Cron)
	default:
		said = fmt.Sprintf("on a schedule of the unknown kind %q", schedule.Kind)
	}
	if zone := oneLine(schedule.Timezone); zone != "" {
		said += " in " + zone
	}
	return said
}

// intervalInWords says a length of time the way a person says it, using the
// largest unit it divides into exactly, so that two hours is "2 hours" and not
// "7200 seconds".
func intervalInWords(interval time.Duration) string {
	if interval <= 0 {
		return "no time at all"
	}
	for _, unit := range []struct {
		length time.Duration
		name   string
	}{
		{24 * time.Hour, "day"},
		{time.Hour, "hour"},
		{time.Minute, "minute"},
		{time.Second, "second"},
	} {
		if interval%unit.length != 0 {
			continue
		}
		count := int64(interval / unit.length)
		if count == 1 {
			return unit.name
		}
		return fmt.Sprintf("%d %ss", count, unit.name)
	}
	return interval.String()
}

// timeOfDayInWords says a moment of the day the way a person says it, so that
// seven in the morning reads as "7 in the morning" rather than as "07:00".
func timeOfDayInWords(hour int, minute int) string {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return fmt.Sprintf("%02d:%02d", hour, minute)
	}
	if minute == 0 && hour == 0 {
		return "midnight"
	}
	if minute == 0 && hour == 12 {
		return "noon"
	}
	onTheClock := hour % 12
	if onTheClock == 0 {
		onTheClock = 12
	}
	partOfDay := "in the evening"
	switch {
	case hour < 12:
		partOfDay = "in the morning"
	case hour < 18:
		partOfDay = "in the afternoon"
	}
	if minute == 0 {
		return fmt.Sprintf("%d %s", onTheClock, partOfDay)
	}
	return fmt.Sprintf("%d:%02d %s", onTheClock, minute, partOfDay)
}

// dueInPlainWords is the date a task must wait for, written into its line in the
// job record. It holds no comma, because a comma is where the record's own line
// splits the task from its date.
func dueInPlainWords(dueAt time.Time) string {
	if dueAt.IsZero() {
		return ""
	}
	return "on " + dueAt.Format("2 January 2006 at 15:04 MST")
}

// momentInPlainWords is a moment written for a listing, and "never" when there
// is no moment at all.
func momentInPlainWords(moment time.Time) string {
	if moment.IsZero() {
		return "never"
	}
	return moment.Format("2006-01-02 15:04")
}

// listInWords joins names the way a sentence does, with "and" before the last
// one, so that three days read as "Monday, Wednesday and Friday".
func listInWords(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

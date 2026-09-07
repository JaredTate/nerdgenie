package clock

import (
	"strconv"
	"time"
)

// Words writes how long something took the way a person reads it: seconds
// under a minute, minutes and seconds under an hour, hours and minutes after
// that, with a zero part left off. A span below nothing, which a clock set
// back can make, is written as nothing at all. It is the one way the harness
// says what a task or a job took, in a report and on the screen.
func Words(span time.Duration) string {
	if span < 0 {
		span = 0
	}
	hours := int(span / time.Hour)
	minutes := int((span % time.Hour) / time.Minute)
	seconds := int((span % time.Minute) / time.Second)
	switch {
	case hours > 0:
		return withPart(strconv.Itoa(hours)+"h", minutes, "m")
	case minutes > 0:
		return withPart(strconv.Itoa(minutes)+"m", seconds, "s")
	default:
		return strconv.Itoa(seconds) + "s"
	}
}

// withPart adds the smaller part after a space when it is not zero.
func withPart(bigger string, smaller int, unit string) string {
	if smaller == 0 {
		return bigger
	}
	return bigger + " " + strconv.Itoa(smaller) + unit
}

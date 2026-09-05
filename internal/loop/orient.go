package loop

import "strings"

// orient takes the model's first line, which says where the work stands, and
// keeps it for the record's situation.
func (running *run) orient(text string, whole string) {
	said := firstLine(text)
	if said == "" {
		said = firstLine(whole)
	}
	if said != "" {
		running.lastOrient = said
	}
}

// firstLine is the first line of a piece of text with nothing else on it.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

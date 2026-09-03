package edit

import "strings"

// MaxCandidateSpans is how many candidate spans one matcher offers before it
// stops looking. A file of a million identical lines must not turn one edit into
// a million comparisons.
const MaxCandidateSpans = 200

// exactSpans offers the text exactly as the model wrote it.
func exactSpans(_ string, wanted string) []string {
	return []string{wanted}
}

// trimmedLineSpans offers the runs of lines that match the wanted text once the
// trailing spaces are taken off both sides. It is the matcher for a file whose
// lines carry trailing whitespace the model did not copy, and for a file whose
// lines end differently from the model's copy of them.
func trimmedLineSpans(content string, wanted string) []string {
	lines := strings.Split(content, "\n")
	wantedLines := strings.Split(wanted, "\n")
	found := []string{}

	for at := 0; at+len(wantedLines) <= len(lines) && len(found) < MaxCandidateSpans; at++ {
		block := lines[at : at+len(wantedLines)]
		if sameOnce(block, wantedLines, trimTrailing) {
			found = append(found, strings.Join(block, "\n"))
		}
	}
	return found
}

// whitespaceNormalizedSpans offers the whole lines that match the wanted text
// once every run of whitespace in each counts as one space. It is the matcher
// for a line the model wrote with different spacing inside it, and it works on
// one line only, because a whole block normalized this way would match far too
// much.
func whitespaceNormalizedSpans(content string, wanted string) []string {
	if strings.Contains(strings.TrimSpace(wanted), "\n") {
		return nil
	}
	flattened := normalizeWhitespace(wanted)
	if flattened == "" {
		return nil
	}

	found := []string{}
	for _, line := range strings.Split(content, "\n") {
		if len(found) >= MaxCandidateSpans {
			break
		}
		if normalizeWhitespace(line) == flattened {
			found = append(found, line)
		}
	}
	return found
}

// indentationFlexibleSpans offers the blocks that match the wanted text once the
// indentation the whole of each shares is taken off both sides. It is the
// matcher for a block the model quoted from one nesting level and the file holds
// at another.
func indentationFlexibleSpans(content string, wanted string) []string {
	lines := strings.Split(content, "\n")
	wantedLines := strings.Split(wanted, "\n")
	flattened := withoutSharedIndent(wantedLines)
	found := []string{}

	for at := 0; at+len(wantedLines) <= len(lines) && len(found) < MaxCandidateSpans; at++ {
		block := lines[at : at+len(wantedLines)]
		if strings.Join(withoutSharedIndent(block), "\n") == strings.Join(flattened, "\n") {
			found = append(found, strings.Join(block, "\n"))
		}
	}
	return found
}

// uniqueSubstringSpans offers the wanted text with the whitespace at its ends
// taken off. It is the matcher for a model that quoted the middle of a line with
// a space or a line break on either side of what it meant.
func uniqueSubstringSpans(_ string, wanted string) []string {
	trimmed := strings.TrimSpace(wanted)
	if trimmed == "" || trimmed == wanted {
		return nil
	}
	return []string{trimmed}
}

// sameOnce says whether two runs of lines are the same once each line has been
// put through the same tidying.
func sameOnce(block []string, wanted []string, tidy func(string) string) bool {
	if len(block) != len(wanted) {
		return false
	}
	for at := range block {
		if tidy(block[at]) != tidy(wanted[at]) {
			return false
		}
	}
	return true
}

// trimTrailing takes the spaces and the carriage return off the end of a line.
func trimTrailing(line string) string {
	return strings.TrimRight(line, " \t\r")
}

// normalizeWhitespace turns every run of whitespace into one space and takes the
// ends off, so that two lines written with different spacing read the same.
func normalizeWhitespace(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

// withoutSharedIndent takes off the indentation every line with anything on it
// shares, so that a block quoted at one nesting level matches the same block at
// another.
func withoutSharedIndent(lines []string) []string {
	shared := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if shared < 0 || indent < shared {
			shared = indent
		}
	}
	if shared <= 0 {
		return lines
	}

	shorter := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" || len(line) < shared {
			shorter = append(shorter, line)
			continue
		}
		shorter = append(shorter, line[shared:])
	}
	return shorter
}

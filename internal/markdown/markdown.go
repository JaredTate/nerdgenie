package markdown

import "strings"

// Section is one part of a Markdown text: the heading line's level and words,
// and everything between it and the next heading.
type Section struct {
	// Level is the number of hash marks on the heading line, one to six.
	Level int
	// Heading is the heading's words with the hash marks and spaces taken off.
	Heading string
	// Body is the text under the heading, up to the next heading of any level.
	Body string
}

// Sections cuts the text at every heading outside a code fence and returns the
// sections in order. Text before the first heading belongs to no section. A
// text with no heading has no sections.
func Sections(text string) []Section {
	var sections []Section
	var body strings.Builder
	inFence := false
	open := false
	closeSection := func() {
		if open {
			sections[len(sections)-1].Body = body.String()
			body.Reset()
		}
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(strings.TrimSpace(trimmed), "```") {
			inFence = !inFence
		}
		if level, heading, isHeading := headingOf(trimmed); isHeading && !inFence {
			closeSection()
			sections = append(sections, Section{Level: level, Heading: heading})
			open = true
			continue
		}
		if open {
			body.WriteString(line)
		}
	}
	closeSection()
	return sections
}

// headingOf reads a heading line: one to six hash marks, a space, and words.
func headingOf(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	heading := strings.TrimSpace(line[level+1:])
	if heading == "" {
		return 0, "", false
	}
	return level, heading, true
}

// Find finds the section whose heading matches, without regard to case,
// surrounding spaces, or leading hash marks, and says whether it found one.
func Find(text string, heading string) (Section, bool) {
	wanted := normalise(heading)
	for _, section := range Sections(text) {
		if normalise(section.Heading) == wanted {
			return section, true
		}
	}
	return Section{}, false
}

// Headings lists the headings of the text, in order.
func Headings(text string) []string {
	var headings []string
	for _, section := range Sections(text) {
		headings = append(headings, section.Heading)
	}
	return headings
}

// ReplaceSection puts a new body under the heading and returns the whole text
// with everything else untouched; when the heading is not there, the section
// is appended at the end as a level-two heading, and found is false. The body
// is written after one blank line and ends with a newline.
func ReplaceSection(text string, heading string, body string) (string, bool) {
	body = strings.TrimRight(body, "\n") + "\n"
	sections := Sections(text)
	wanted := normalise(heading)
	for index, section := range sections {
		if normalise(section.Heading) != wanted {
			continue
		}
		var out strings.Builder
		out.WriteString(textBeforeFirstHeading(text))
		for at, each := range sections {
			out.WriteString(strings.Repeat("#", each.Level) + " " + each.Heading + "\n")
			if at == index {
				out.WriteString("\n" + body)
				if at < len(sections)-1 {
					out.WriteString("\n")
				}
				continue
			}
			out.WriteString(each.Body)
		}
		return out.String(), true
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	return text + "## " + strings.TrimSpace(heading) + "\n\n" + body, false
}

// textBeforeFirstHeading is what comes before the first heading outside a
// fence, kept as it was when a section is replaced.
func textBeforeFirstHeading(text string) string {
	inFence := false
	var out strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(strings.TrimSpace(trimmed), "```") {
			inFence = !inFence
		}
		if _, _, isHeading := headingOf(trimmed); isHeading && !inFence {
			return out.String()
		}
		out.WriteString(line)
	}
	return out.String()
}

// normalise makes two headings comparable: hash marks and spaces off, lower case.
func normalise(heading string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(heading), "#")))
}

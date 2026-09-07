package codemap

import (
	"path"
	"regexp"
	"strings"
)

const (
	// MaxNamesPerFile is the most names one file's entry lists; a file past
	// it says how many more there are.
	MaxNamesPerFile = 60
	// MaxSentenceRunes is the longest a name's one-line description may be.
	MaxSentenceRunes = 160
	// MaxSourceBytes is the most of one file that is read for its names.
	MaxSourceBytes = 512 << 10
)

// Name is one thing a file defines.
type Name struct {
	// Kind is func, method, class, type or const.
	Kind string
	// Name is the bare identifier.
	Name string
	// Signature is the name with its parameters when it has them, such as
	// "spawn(board, piece)", and the bare name otherwise.
	Signature string
	// Says is the first sentence of the comment above the definition, or
	// nothing when there is none.
	Says string
}

// File is one file's entry in the map.
type File struct {
	// Path is the file's path as given, with forward slashes.
	Path string
	// IsCode says whether the file is in a language the map reads.
	IsCode bool
	// IsTest says whether the file is a test file, by its name or folder.
	IsTest bool
	// FirstLine is the comment at the top of the file, its first sentence.
	FirstLine string
	// Names are the things the file defines, in order, at most MaxNamesPerFile.
	Names []Name
	// More is how many names were left out past the cap.
	More int
	// Tests is how many test cases the file holds, for a test file.
	Tests int
}

// language is how one family of languages is read.
type language struct {
	// defs find a definition on one line: the groups are the kind's marker
	// text, the name, and the parameters, as each pattern arranges them.
	defs []definer
	// comment says whether a line is a comment line and gives its text.
	comment func(line string) (string, bool)
	// test counts the test cases on one line.
	test *regexp.Regexp
	// docstringBelow says the description sits on the line after the
	// definition, as a Python docstring does.
	docstringBelow bool
}

// definer is one pattern and how to read a name off its match.
type definer struct {
	pattern *regexp.Regexp
	kind    string
	name    int
	params  int
	// nested says the pattern matches only indented lines, which are the
	// methods of the class above them.
	nested bool
}

// Read reads one file's names. A file whose extension the map does not know
// is listed by its path alone.
func Read(filePath string, source []byte) File {
	file := File{Path: strings.ReplaceAll(filePath, "\\", "/")}
	file.IsTest = looksLikeATestFile(file.Path)
	spoken, known := languageOf(path.Ext(file.Path))
	if !known {
		return file
	}
	file.IsCode = true
	if len(source) > MaxSourceBytes {
		source = source[:MaxSourceBytes]
	}
	lines := strings.Split(string(source), "\n")
	file.FirstLine = firstCommentOf(lines, spoken)
	insideAClass := false
	for at, line := range lines {
		if spoken.test != nil {
			file.Tests += len(spoken.test.FindAllStringIndex(line, -1))
		}
		name, found := spoken.nameOn(line, insideAClass)
		if !found {
			continue
		}
		if name.Kind == "class" || name.Kind == "type" {
			insideAClass = name.Kind == "class"
		}
		name.Says = spoken.descriptionAround(lines, at)
		if len(file.Names) >= MaxNamesPerFile {
			file.More++
			continue
		}
		file.Names = append(file.Names, name)
	}
	return file
}

// nameOn reads a definition off one line, if the line holds one.
func (spoken language) nameOn(line string, insideAClass bool) (Name, bool) {
	for _, def := range spoken.defs {
		match := def.pattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name := match[def.name]
		if name == "" || isAKeyword(name) {
			continue
		}
		kind := def.kind
		if def.nested && !insideAClass {
			// An indented function outside a class is a local helper or a
			// callback, not a name the map lists.
			continue
		}
		signature := name
		if def.params > 0 && def.params < len(match) {
			signature = name + "(" + strings.TrimSpace(match[def.params]) + ")"
		}
		return Name{Kind: kind, Name: name, Signature: signature}, true
	}
	return Name{}, false
}

// descriptionAround is the first sentence of the comment directly above the
// definition, or of the docstring directly below it in a language that puts
// it there.
func (spoken language) descriptionAround(lines []string, at int) string {
	if spoken.docstringBelow && at+1 < len(lines) {
		if said, ok := docstringOn(lines[at+1]); ok {
			return firstSentence(said)
		}
	}
	var above []string
	for back := at - 1; back >= 0; back-- {
		text, ok := spoken.comment(lines[back])
		if !ok {
			break
		}
		above = append([]string{text}, above...)
	}
	return firstSentence(strings.Join(above, " "))
}

// firstCommentOf is the first sentence of the comment or docstring at the very
// top of the file, skipping a shebang and blank lines.
func firstCommentOf(lines []string, spoken language) string {
	var top []string
	for at, line := range lines {
		trimmed := strings.TrimSpace(line)
		if at == 0 && strings.HasPrefix(trimmed, "#!") {
			continue
		}
		if trimmed == "" && len(top) == 0 {
			continue
		}
		if said, ok := docstringOn(line); ok && len(top) == 0 && spoken.docstringBelow {
			return firstSentence(said)
		}
		text, ok := spoken.comment(line)
		if !ok {
			break
		}
		top = append(top, text)
	}
	return firstSentence(strings.Join(top, " "))
}

// firstSentence cuts a comment to its first sentence, with the markers and
// tags a doc comment carries taken off, bounded in length.
func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	for _, tag := range []string{"@param", "@return", "@throws", "@type", "@template"} {
		if at := strings.Index(text, tag); at >= 0 {
			text = strings.TrimSpace(text[:at])
		}
	}
	if at := strings.Index(text, ". "); at >= 0 {
		text = text[:at+1]
	}
	if runes := []rune(text); len(runes) > MaxSentenceRunes {
		text = string(runes[:MaxSentenceRunes-3]) + "..."
	}
	return text
}

// docstringOn reads the first line of a triple-quoted docstring.
func docstringOn(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, quote := range []string{`"""`, `'''`} {
		if strings.HasPrefix(trimmed, quote) {
			inner := strings.TrimPrefix(trimmed, quote)
			inner = strings.TrimSuffix(inner, quote)
			return strings.TrimSpace(inner), true
		}
	}
	return "", false
}

// looksLikeATestFile says whether a path names a test file, by its name or by
// a test folder on its way.
func looksLikeATestFile(filePath string) bool {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	if strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_spec.rb") {
		return true
	}
	for _, folder := range strings.Split(path.Dir(lower), "/") {
		switch folder {
		case "test", "tests", "spec", "specs", "__tests__":
			return true
		}
	}
	return false
}

// keywords are the words a definition pattern can match that are not names.
var keywords = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "return": true, "else": true, "catch": true, "sizeof": true, "defined": true, "function": true, "new": true, "delete": true, "typeof": true, "await": true, "yield": true, "do": true, "try": true, "with": true, "and": true, "or": true, "not": true}

func isAKeyword(name string) bool { return keywords[strings.ToLower(name)] }

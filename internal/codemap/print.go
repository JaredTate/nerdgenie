package codemap

import (
	"fmt"
	"sort"
	"strings"
)

// GeneratedMark is the line that says the harness wrote the map, and is what
// lets it write the map again; a map without it is a person's own.
const GeneratedMark = "<!-- generated: nerdgenie -->"

// Print writes the map of a folder from its files: the title, the mark, a
// legend of the top folders, the source files with their names, the tests
// by file with their counts, and the other files as a plain list.
func Print(name string, files []File) string {
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	var out strings.Builder
	fmt.Fprintf(&out, "# Repository Map: %s\n\n%s\n\nThis file is written by Nerd Genie at the end of every task of a job here. Do not edit it by hand; remove the mark above to keep your own. Find a file's functions with `read REPO_MAP.md <path>` before searching or reading for them.\n\n", name, GeneratedMark)
	printLegend(&out, files)
	var source, tests, other []File
	for _, file := range files {
		switch {
		case file.IsCode && file.IsTest:
			tests = append(tests, file)
		case file.IsCode:
			source = append(source, file)
		default:
			other = append(other, file)
		}
	}
	if len(source) > 0 {
		out.WriteString("## Source files\n\n")
		for _, file := range source {
			printEntry(&out, file)
		}
	}
	if len(tests) > 0 {
		out.WriteString("## Tests\n\n")
		for _, file := range tests {
			printTest(&out, file)
		}
	}
	if len(other) > 0 {
		out.WriteString("## Other files\n\n```text\n")
		for _, file := range other {
			out.WriteString(file.Path + "\n")
		}
		out.WriteString("```\n")
	}
	return out.String()
}

// printLegend writes the Roots section: one line per top folder with its
// file count.
func printLegend(out *strings.Builder, files []File) {
	counts := map[string]int{}
	var roots []string
	for _, file := range files {
		root := "./"
		if at := strings.Index(file.Path, "/"); at >= 0 {
			root = file.Path[:at] + "/"
		}
		if counts[root] == 0 {
			roots = append(roots, root)
		}
		counts[root]++
	}
	sort.Strings(roots)
	out.WriteString("## Roots\n\n")
	for _, root := range roots {
		fmt.Fprintf(out, "- `%s` - %d files\n", root, counts[root])
	}
	out.WriteString("\n")
}

// EntryBody is what a file's entry says under its heading: its first line and
// one line per name for a source file, its first line and its count for a
// test file. It is what a write puts back into the map for the one file it
// changed.
func EntryBody(file File) string {
	var out strings.Builder
	if file.IsCode && file.IsTest {
		printTest(&out, file)
	} else {
		printEntry(&out, file)
	}
	body := out.String()
	if at := strings.Index(body, "\n"); at >= 0 {
		body = body[at+1:]
	}
	return strings.TrimRight(body, "\n") + "\n"
}

// printEntry writes one source file: its heading, its first line, and one
// line per name with methods indented under their class.
func printEntry(out *strings.Builder, file File) {
	fmt.Fprintf(out, "### %s\n", file.Path)
	if file.FirstLine != "" {
		out.WriteString(file.FirstLine + "\n")
	}
	for _, name := range file.Names {
		indent := "- "
		if name.Kind == "method" {
			indent = "  - "
		}
		label := "`" + name.Signature + "`"
		switch name.Kind {
		case "class", "type", "const":
			label += " (" + name.Kind + ")"
		}
		if name.Says != "" {
			label += " → " + name.Says
		}
		out.WriteString(indent + label + "\n")
	}
	if file.More > 0 {
		fmt.Fprintf(out, "- and %d more\n", file.More)
	}
	if len(file.Names) == 0 && file.More == 0 {
		// A code file the reader found nothing in says so, so that a gap in
		// the map is seen and not mistaken for an empty file.
		out.WriteString("- no names read\n")
	}
	out.WriteString("\n")
}

// printTest writes one test file: its heading, its first line, and its count.
func printTest(out *strings.Builder, file File) {
	fmt.Fprintf(out, "### %s\n", file.Path)
	if file.FirstLine != "" {
		out.WriteString(file.FirstLine + "\n")
	}
	noun := "tests"
	if file.Tests == 1 {
		noun = "test"
	}
	fmt.Fprintf(out, "- %d %s\n\n", file.Tests, noun)
}

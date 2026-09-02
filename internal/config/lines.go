package config

import (
	"strconv"
	"strings"
)

// keyLine remembers the line each key in the configuration file is written on,
// so that a message about a key can point at the place to fix it. The TOML
// library reports a position for a line it cannot parse, but not for a key it
// parsed and then found nothing to put it in, and not for a value this package
// checks itself, so those lines are found here.
type keyLine map[string]int

// keyLines reads a configuration file and notes the line every key is written
// on. Keys inside a table carry the table's name, so a "rounds_per_task" line
// under a "[caps]" header is noted as "caps.rounds_per_task", and the entries of a
// table array are numbered, so the first "[[models]]" block is "models.0" and a
// key inside it is noted both with the number and without it. This
// reads the file as lines rather than as TOML, so a key inside a value spread
// over several lines may be noted as well; that is harmless, because only keys
// this package already knows about are ever looked up.
func keyLines(document string) keyLine {
	lines := keyLine{}
	table := ""
	entries := map[string]int{}
	for offset, text := range strings.Split(document, "\n") {
		number := offset + 1
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "[["):
			name := tableName(trimmed, "[[", "]]")
			if name == "" {
				continue
			}
			table = name + "." + strconv.Itoa(entries[name])
			entries[name]++
			lines.note(name, number)
			lines.note(table, number)
		case strings.HasPrefix(trimmed, "["):
			name := tableName(trimmed, "[", "]")
			if name == "" {
				continue
			}
			table = name
			lines.note(name, number)
		default:
			written, _, found := strings.Cut(trimmed, "=")
			if !found {
				continue
			}
			key := cleanKey(written)
			lines.note(joinKey(table, key), number)
			lines.note(joinKey(withoutTheNumber(table), key), number)
		}
	}
	return lines
}

// withoutTheNumber takes the entry number off a table array's name, so that a
// key inside the first "[[models]]" block is noted both as "models.0.name" and
// as "models.name". The TOML library reports an unknown key inside a table array
// without the number, and the checks in this package use the number to say which
// block is wrong, so both forms have to lead back to the same line.
func withoutTheNumber(table string) string {
	name, entry, found := strings.Cut(table, ".")
	if !found {
		return table
	}
	if _, err := strconv.Atoi(entry); err != nil {
		return table
	}
	return name
}

// of returns the line a key is written on, or zero when the file does not say.
// A key the file never names falls back to the line of the table it would live
// in, so a missing "context_length" inside a "[[models]]" block still points at
// that block.
func (lines keyLine) of(key string) int {
	wanted := strings.ToLower(key)
	for wanted != "" {
		if number, found := lines[wanted]; found {
			return number
		}
		cut := strings.LastIndex(wanted, ".")
		if cut < 0 {
			return 0
		}
		wanted = wanted[:cut]
	}
	return 0
}

// note writes down the first line a key appears on, because a key written twice
// is a mistake the TOML library reports at the second one.
func (lines keyLine) note(key string, number int) {
	if key == "" {
		return
	}
	if _, already := lines[key]; already {
		return
	}
	lines[key] = number
}

// tableName reads the name out of a table header such as "[caps]" or
// "[[models]] # the aliases", and returns an empty string when the line turns
// out not to be a header after all.
func tableName(line string, opener string, closer string) string {
	rest, found := strings.CutPrefix(line, opener)
	if !found {
		return ""
	}
	name, _, found := strings.Cut(rest, closer)
	if !found {
		return ""
	}
	return cleanKey(name)
}

// cleanKey takes the spaces and the quotes off a key as it is written in the
// file and lower-cases it, because the decoder matches a key to a field name
// without regard to case.
func cleanKey(written string) string {
	pieces := strings.Split(written, ".")
	for at, piece := range pieces {
		pieces[at] = strings.TrimSpace(strings.Trim(strings.TrimSpace(piece), `"'`))
	}
	return strings.ToLower(strings.Join(pieces, "."))
}

// joinKey puts a table's name in front of a key inside it.
func joinKey(prefix string, key string) string {
	switch {
	case prefix == "":
		return key
	case key == "":
		return prefix
	default:
		return prefix + "." + key
	}
}

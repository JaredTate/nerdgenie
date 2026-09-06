package orientation

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The bounds on the block.
const (
	// MaxNames is how many entries of the working folder are named.
	MaxNames = 40
	// MaxPorts is how many listening ports are named.
	MaxPorts = 20
	// MaxResultLetters is how much of one result is shown, head and tail.
	MaxResultLetters = 1500
	// MaxBlockLetters is the most the whole block may be.
	MaxBlockLetters = 6000
)

// TheHeading opens the block.
const TheHeading = "Where things stand on this machine:"

// TheResultsHeading opens the newest results, shown only on a window that is
// fresh for a task that is not.
const TheResultsHeading = "the newest results, in full:"

// TheProcFiles are where Linux lists its TCP sockets.
var TheProcFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// Result is one past result of the task, whole.
type Result struct {
	// ID is the result's label in the record, such as "r41".
	ID string
	// Text is the whole of what the tool answered.
	Text string
}

// Facts is what the block is built from.
type Facts struct {
	// Folder is the folder the agent works in, or empty to leave it out.
	Folder string
	// ProcFiles are the socket tables to read the listening ports from; nil
	// means TheProcFiles.
	ProcFiles []string
	// Results are the newest results of the task, newest last, or none.
	Results []Result
}

// Block writes the facts as plain lines under the heading. Nothing here can
// fail the task: what cannot be read is one line saying so.
func Block(facts Facts) string {
	lines := []string{TheHeading}
	if facts.Folder != "" {
		lines = append(lines, folderLines(facts.Folder)...)
	}
	files := facts.ProcFiles
	if files == nil {
		files = TheProcFiles
	}
	lines = append(lines, portsLine(files))
	if len(facts.Results) > 0 {
		lines = append(lines, TheResultsHeading)
		for _, result := range facts.Results {
			lines = append(lines, result.ID+":", cutKeepingBothEnds(result.Text, MaxResultLetters))
		}
	}
	return cutKeepingBothEnds(strings.Join(lines, "\n"), MaxBlockLetters)
}

// folderLines names what is in the folder: sorted, folders with a trailing
// slash, capped, with a last line saying how many more there are.
func folderLines(folder string) []string {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return []string{fmt.Sprintf("the working folder %s could not be read: %v", folder, err)}
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return []string{fmt.Sprintf("the working folder %s is empty", folder)}
	}
	lines := []string{fmt.Sprintf("in the working folder %s:", folder)}
	shown := names
	if len(shown) > MaxNames {
		shown = shown[:MaxNames]
	}
	lines = append(lines, "  "+strings.Join(shown, "  "))
	if len(names) > MaxNames {
		lines = append(lines, fmt.Sprintf("  ... and %d more", len(names)-MaxNames))
	}
	return lines
}

// portsLine names the TCP ports listening on this machine, sorted, capped.
func portsLine(files []string) string {
	ports, problems := ListeningPorts(files...)
	words := []string{}
	for _, port := range ports {
		words = append(words, strconv.Itoa(port))
	}
	line := "listening ports: none"
	if len(words) > 0 {
		line = "listening ports: " + strings.Join(words, " ")
	}
	if len(problems) > 0 {
		line += " (" + strings.Join(problems, "; ") + ")"
	}
	return line
}

// ListeningPorts reads the ports in the LISTEN state out of Linux's socket
// tables, each port once, sorted, at most MaxPorts of them, and says which
// tables could not be read.
func ListeningPorts(files ...string) ([]int, []string) {
	seen := map[int]bool{}
	problems := []string{}
	for _, file := range files {
		opened, err := os.Open(file)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s could not be read", file))
			continue
		}
		lines := bufio.NewScanner(opened)
		for lines.Scan() {
			if port, listening := listeningPortIn(lines.Text()); listening {
				seen[port] = true
			}
		}
		_ = opened.Close()
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	slices.Sort(ports)
	if len(ports) > MaxPorts {
		ports = ports[:MaxPorts]
	}
	return ports, problems
}

// listeningPortIn reads one row of a socket table: the local address is the
// second field as a hex address, a colon, and a hex port, and the state is the
// fourth field, 0A for LISTEN.
func listeningPortIn(row string) (int, bool) {
	fields := strings.Fields(row)
	if len(fields) < 4 || fields[3] != "0A" {
		return 0, false
	}
	_, portHex, found := strings.Cut(fields[1], ":")
	if !found {
		return 0, false
	}
	port, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil || port <= 0 {
		return 0, false
	}
	return int(port), true
}

// cutKeepingBothEnds cuts text to the limit with its head and its tail kept
// and a middle line saying how much was cut, because the end of a result is
// where an error or a total lives and the start is where the command is.
func cutKeepingBothEnds(text string, most int) string {
	if len(text) <= most {
		return text
	}
	half := most / 2
	cut := len(text) - most
	return text[:half] + fmt.Sprintf("\n... %d characters cut ...\n", cut) + text[len(text)-half:]
}

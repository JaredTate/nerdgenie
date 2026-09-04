package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxConfigBytes is the largest configuration file Nerd Genie will read. A settings
// file is a page or two, so anything past this is a mistake or a file that is
// not a configuration at all, and reading it whole into memory helps nobody.
const MaxConfigBytes = 1 << 20

// Problem is one thing wrong with the configuration: which key, which line of
// the file when the file says, and what to do about it.
type Problem struct {
	// Path is the configuration file the problem is in.
	Path string
	// Line is the line the problem is on, or zero when nothing says.
	Line int
	// Key is the dotted key, such as "caps.rounds_per_task", or empty when the
	// problem belongs to the file as a whole.
	Key string
	// Advice says what is wrong and what to do, in one sentence.
	Advice string
}

// Error prints the problem as "path:line: key: what to do", which is the shape
// an editor can jump to.
func (problem Problem) Error() string {
	where := problem.Path
	if problem.Line > 0 {
		where = problem.Path + ":" + strconv.Itoa(problem.Line)
	}
	if problem.Key == "" {
		return where + ": " + problem.Advice
	}
	return where + ": " + problem.Key + ": " + problem.Advice
}

// Load reads a home folder's configuration file, checks it, and returns the
// configuration the rest of the program runs on. A missing file means the
// defaults, because a fresh install has nothing to say yet.
func Load(home contract.Home) (contract.Config, error) {
	path := home.ConfigFile()
	about, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Parse(home, nil)
	case err != nil:
		return contract.Config{}, fmt.Errorf("cannot look at the configuration file %s, so check that the home folder is readable: %w", path, err)
	case about.Size() > MaxConfigBytes:
		return contract.Config{}, Problem{Path: path, Advice: fmt.Sprintf(
			"this file is %d bytes and the limit is %d, so it is not a settings file; move it aside and let nerdgenie init write a new one",
			about.Size(), MaxConfigBytes)}
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return contract.Config{}, fmt.Errorf("cannot read the configuration file %s, so check that it is readable by this account: %w", path, err)
	}
	return Parse(home, document)
}

// Parse reads configuration text as though it had come from the home folder's
// configuration file. It starts from the defaults, decodes the text over them,
// fills in the paths that depend on where the home folder is, and then checks
// every field, so that an empty document is a valid configuration.
func Parse(home contract.Home, document []byte) (contract.Config, error) {
	path := home.ConfigFile()
	if len(document) > MaxConfigBytes {
		return contract.Config{}, Problem{Path: path, Advice: fmt.Sprintf(
			"this configuration is %d bytes and the limit is %d, so it is not a settings file", len(document), MaxConfigBytes)}
	}

	settings := contract.DefaultConfig()
	text := string(document)
	lines := keyLines(text)
	written, err := toml.Decode(text, &settings)
	if err != nil {
		return contract.Config{}, problemFromDecodeError(path, err)
	}
	if unknown := firstUnknownKey(written.Undecoded(), lines); unknown != "" {
		return contract.Config{}, Problem{Path: path, Line: lines.of(unknown), Key: unknown, Advice: "the configuration has no such key, so check the spelling against the toml tags in internal/contract.Config, which write every key in lower case with underscores between the words"}
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		return contract.Config{}, errors.New("cannot find your home directory, which the sandbox roots are measured from, so set the HOME environment variable and try again")
	}
	fillPathsFromTheHome(&settings, home, userHome, lines)

	if err := (settingsChecker{settings: settings, path: path, lines: lines, userHome: userHome, agentHome: home.Root}).run(); err != nil {
		return contract.Config{}, err
	}
	return settings, nil
}

// fillPathsFromTheHome sets the three fields whose default is not a fixed value
// but a place: the browser profile, the backup folder, and the sandbox roots all
// depend on where the home folder and the user's home directory are, which
// contract.DefaultConfig cannot know. A key the file writes for itself is left
// alone even when what it wrote is empty, so that a line saying "sandbox_roots =
// []" means what it says and is refused rather than quietly turned into the
// default.
func fillPathsFromTheHome(settings *contract.Config, home contract.Home, userHome string, lines keyLine) {
	if settings.BrowserProfilePath == "" && lines.of("browser_profile_path") == 0 {
		settings.BrowserProfilePath = home.BrowserProfile("default")
	}
	if settings.BackupPath == "" && lines.of("backup_path") == 0 {
		settings.BackupPath = home.BackupsFolder()
	}
	if len(settings.SandboxRoots) == 0 && lines.of("sandbox_roots") == 0 {
		settings.SandboxRoots = contract.DefaultSandboxRoots(userHome)
	}
}

// firstUnknownKey picks the key to complain about out of everything the decoder
// could not place: the one written earliest in the file, so that a person fixing
// the file works downwards.
func firstUnknownKey(undecoded []toml.Key, lines keyLine) string {
	names := make([]string, 0, len(undecoded))
	for _, key := range undecoded {
		names = append(names, key.String())
	}
	slices.SortFunc(names, func(left, right string) int {
		if leftLine, rightLine := lines.of(left), lines.of(right); leftLine != rightLine {
			return leftLine - rightLine
		}
		return strings.Compare(left, right)
	})
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// problemFromDecodeError turns what the TOML library reports into a Problem, so
// that every message about the configuration file has the same shape. The
// library gives a position for text it cannot parse; for a value of the wrong
// type it gives only a sentence beginning "toml: line 4 (last key \"x\"): ",
// and readDecodeMessage reads the line and the key back out of that.
func problemFromDecodeError(path string, err error) Problem {
	var parsing toml.ParseError
	if errors.As(err, &parsing) {
		return Problem{
			Path:   path,
			Line:   parsing.Position.Line,
			Key:    parsing.LastKey,
			Advice: tidy(parsing.Message) + ", so correct this line and try again",
		}
	}
	line, key, message := readDecodeMessage(err.Error())
	return Problem{Path: path, Line: line, Key: key, Advice: tidy(message) + ", so give the key a value of the kind it expects"}
}

// readDecodeMessage reads the line number and the key back out of the sentence
// the TOML library builds for a value it cannot use. A sentence in any other
// shape comes back whole, with no line and no key, because a message that says
// something is better than a message that says nothing.
func readDecodeMessage(sentence string) (int, string, string) {
	rest, found := strings.CutPrefix(sentence, "toml: line ")
	if !found {
		return 0, "", strings.TrimPrefix(sentence, "toml: ")
	}
	digits, rest, found := strings.Cut(rest, " (last key ")
	if !found {
		return 0, "", sentence
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return 0, "", sentence
	}
	quoted, message, found := strings.Cut(rest, "): ")
	if !found {
		return number, "", sentence
	}
	key, err := strconv.Unquote(quoted)
	if err != nil {
		key = strings.Trim(quoted, `"`)
	}
	return number, key, message
}

// tidy takes the full stop off the end of a message written by the TOML library,
// so that it reads as part of a longer sentence.
func tidy(message string) string {
	return strings.TrimSuffix(strings.TrimSpace(message), ".")
}

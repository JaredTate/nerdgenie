// The folder format is Hermes' skill format written fresh for Coeus, at
// ~/Code/hermes-agent/tools/skills_tool.py: SKILL.md is the one file a folder
// cannot do without, a listing reads only its head, and the name is a lowercase
// hyphenated word of at most sixty-four characters. The rule that a name holds
// only lowercase letters, digits, and inner hyphens is ZeroClaw's slug check at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/skills/creator.rs.

package skill

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/JaredTate/coeus/internal/contract"
)

// The names of the four files a skill folder holds. They are fixed rather than
// configurable, so that a folder somebody shares works on anybody's machine.
const (
	// DescriptionFile is the file holding the name, the one-line description,
	// the trigger words, and the permissions block.
	DescriptionFile = "SKILL.md"
	// StepsFile is the file holding the numbered steps.
	StepsFile = "steps.md"
	// ScriptFile is the executable a skill may carry in place of steps.
	ScriptFile = "script"
	// TestFile is the file holding the dry run's arguments and what it expects.
	TestFile = "test.md"
	// ChangelogFile is the file holding every change with a way to undo it.
	ChangelogFile = "CHANGELOG.md"
	// VersionsFolder is the folder inside a skill holding the copy each save
	// replaced, one numbered folder per version.
	VersionsFolder = "versions"
)

// The bounds on a skill folder. Every one of them exists because the file comes
// from outside the program, and something from outside the program with no
// limit on it is a way to fill the disk or the model's context.
const (
	// MaxSkills is how many skills one machine holds.
	MaxSkills = 200
	// MaxNameRunes is how long a skill's name may be.
	MaxNameRunes = 64
	// MaxDescriptionRunes is how long the one line in the prompt may be.
	MaxDescriptionRunes = 200
	// MaxTriggers is how many trigger words one skill may have.
	MaxTriggers = 20
	// MaxTriggerRunes is how long one trigger may be.
	MaxTriggerRunes = 60
	// MaxSites is how many websites a permissions block may name.
	MaxSites = 20
	// MaxSteps is how many steps one skill may have.
	MaxSteps = 50
	// MaxStepBytes is how long the text of one step may be.
	MaxStepBytes = 4096
	// MaxFileBytes is how big any one file in a skill folder may be.
	MaxFileBytes = 256 * 1024
	// MaxVersions is how many replaced copies are kept beside a skill.
	MaxVersions = 20
	// DefaultDailyLimit is the daily limit a permissions block that names none
	// is given.
	DefaultDailyLimit = 100
	// MaxDailyLimit is the largest daily limit a permissions block may name.
	MaxDailyLimit = 1000
)

// The keys a line of the permissions block may carry. Anything else is a typo,
// and a typo in a permissions block would quietly widen what a skill may do, so
// it is refused by name.
const (
	browserProfileKey   = "browser profile"
	siteKey             = "site"
	dailyLimitKey       = "daily limit"
	irreversibleStepKey = "irreversible step"
)

// Permissions is the block in SKILL.md that says what a skill may do: the
// browser profile it may use, the websites it may visit, how many calls its
// standing approvals may allow in a day, and which of its steps cannot be
// undone.
type Permissions struct {
	// BrowserProfile is the Chrome profile the skill may use, and is empty when
	// the skill does not use a browser.
	BrowserProfile string
	// Sites are the websites the skill may visit. A browser or web step whose
	// address is not one of these is put to the user rather than run.
	Sites []string
	// DailyLimit is how many calls the skill's standing approvals may allow
	// before its steps start asking again, counted from midnight to midnight.
	DailyLimit int
	// IrreversibleSteps are the numbers of the steps that cannot be undone. The
	// dry run stops before the first of them, and a full run previews it.
	IrreversibleSteps []int
}

// Definition is what SKILL.md says: the name, the one line that rides in the
// model's prompt, the words that trigger the skill, and the permissions block.
type Definition struct {
	// Name is the skill's folder name.
	Name string
	// Description is the one line the prompt carries.
	Description string
	// Triggers are the words a message must all hold for the skill to fire.
	Triggers []string
	// Permissions is what the skill may do.
	Permissions Permissions
}

// Summary is the name and the one-liner, which is all the prompt ever sees.
func (definition Definition) Summary() contract.SkillSummary {
	return contract.SkillSummary{Name: definition.Name, Description: definition.Description}
}

// ParseDescriptionFile reads a SKILL.md and returns what it says. It refuses
// anything it cannot read with a message naming the file and what to fix, so
// that a folder somebody hand-edited says what is wrong with it.
func ParseDescriptionFile(content []byte) (Definition, error) {
	if len(content) > MaxFileBytes {
		return Definition{}, fmt.Errorf("%s is %d bytes and the most allowed is %d, so shorten it", DescriptionFile, len(content), MaxFileBytes)
	}

	definition := Definition{Permissions: Permissions{DailyLimit: DefaultDailyLimit}}
	section := ""
	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "## "):
			section = strings.ToLower(strings.TrimSpace(line[3:]))
		case strings.HasPrefix(line, "# "):
			if definition.Name == "" {
				definition.Name = strings.TrimSpace(line[2:])
			}
		default:
			if err := readDescriptionLine(&definition, section, line); err != nil {
				return Definition{}, err
			}
		}
	}
	return definition, checkDefinition(definition)
}

// readDescriptionLine reads one ordinary line of SKILL.md into the definition,
// which depends on the section the line sits in.
func readDescriptionLine(definition *Definition, section string, line string) error {
	switch section {
	case "triggers":
		if trigger := bulletText(line); trigger != "" {
			definition.Triggers = append(definition.Triggers, strings.ToLower(trigger))
		}
		return nil
	case "permissions":
		if written := bulletText(line); written != "" {
			return readPermissionLine(&definition.Permissions, written)
		}
		return nil
	default:
		if definition.Description == "" && line != "" && !strings.HasPrefix(line, "-") {
			definition.Description = line
		}
		return nil
	}
}

// bulletText returns what a "- something" line says, and an empty string for a
// line that is not a bullet.
func bulletText(line string) string {
	if !strings.HasPrefix(line, "- ") {
		return ""
	}
	return strings.TrimSpace(line[2:])
}

// readPermissionLine reads one "key: value" bullet of the permissions block.
func readPermissionLine(permissions *Permissions, written string) error {
	key, value, separated := strings.Cut(written, ":")
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if !separated || value == "" {
		return fmt.Errorf("the permissions line %q in %s says no value, so write it as \"- key: value\"", written, DescriptionFile)
	}

	switch key {
	case browserProfileKey:
		permissions.BrowserProfile = value
	case siteKey:
		if err := checkSite(value); err != nil {
			return fmt.Errorf("the permissions line %q in %s does not name one website, because %w", written, DescriptionFile, err)
		}
		permissions.Sites = append(permissions.Sites, strings.ToLower(value))
	case dailyLimitKey:
		limit, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("the daily limit in %s is %q and it has to be a whole number, so write a number such as 40", DescriptionFile, value)
		}
		permissions.DailyLimit = limit
	case irreversibleStepKey:
		number, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("the irreversible step in %s is %q and it has to be a step number, so write a number such as 3", DescriptionFile, value)
		}
		permissions.IrreversibleSteps = append(permissions.IrreversibleSteps, number)
	default:
		return fmt.Errorf("%s has a permissions line about %q, which is not one of browser profile, site, daily limit, or irreversible step", DescriptionFile, key)
	}
	return nil
}

// checkDefinition holds every bound on what SKILL.md may say, so that the caps
// live in one place rather than scattered through the reader.
func checkDefinition(definition Definition) error {
	if err := CheckName(definition.Name); err != nil {
		return err
	}
	if definition.Description == "" {
		return fmt.Errorf("%s for the skill %q has no description line, so write one line saying what the skill does", DescriptionFile, definition.Name)
	}
	if runes := []rune(definition.Description); len(runes) > MaxDescriptionRunes {
		return fmt.Errorf("the description in %s is %d characters and the most allowed is %d, so shorten it to one line", DescriptionFile, len(runes), MaxDescriptionRunes)
	}
	if len(definition.Triggers) > MaxTriggers {
		return fmt.Errorf("%s names %d triggers and the most allowed is %d, so keep the ones that really pick this skill out", DescriptionFile, len(definition.Triggers), MaxTriggers)
	}
	for _, trigger := range definition.Triggers {
		if len([]rune(trigger)) > MaxTriggerRunes {
			return fmt.Errorf("the trigger %q in %s is longer than %d characters, so shorten it to the words a message would really hold", trigger, DescriptionFile, MaxTriggerRunes)
		}
	}
	return checkPermissions(definition.Permissions)
}

// checkPermissions holds the bounds on the permissions block.
func checkPermissions(permissions Permissions) error {
	if len(permissions.Sites) > MaxSites {
		return fmt.Errorf("the permissions block names %d websites and the most allowed is %d, so name only the sites the skill really visits", len(permissions.Sites), MaxSites)
	}
	for _, site := range permissions.Sites {
		if err := checkSite(site); err != nil {
			return fmt.Errorf("the permissions block names the website %q, which is not one bare host name, because %w", site, err)
		}
	}
	if permissions.DailyLimit < 1 || permissions.DailyLimit > MaxDailyLimit {
		return fmt.Errorf("the daily limit is %d and it has to be between 1 and %d, so write a limit in that range", permissions.DailyLimit, MaxDailyLimit)
	}
	for _, number := range permissions.IrreversibleSteps {
		if number < 1 {
			return fmt.Errorf("an irreversible step is numbered %d and steps start at one, so name the step by its number in %s", number, StepsFile)
		}
	}
	return nil
}

// CheckName says whether a name may be a skill's folder name. A name is
// lowercase letters, digits, and hyphens between them, which is what makes it
// safe as a folder name on every machine and readable in a message.
func CheckName(name string) error {
	if name == "" {
		return fmt.Errorf("a skill needs a name, so put one on the first line of %s as a heading", DescriptionFile)
	}
	if len([]rune(name)) > MaxNameRunes {
		return fmt.Errorf("the skill name %q is longer than %d characters, so give it a shorter name", name, MaxNameRunes)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("the skill name %q starts or ends with a hyphen, so put the hyphens between words instead", name)
	}
	for _, letter := range name {
		if letter == '-' || unicode.IsDigit(letter) || (letter >= 'a' && letter <= 'z') {
			continue
		}
		return fmt.Errorf("the skill name %q holds %q, so use lowercase letters, digits, and hyphens between words", name, string(letter))
	}
	return nil
}

// RenderDescriptionFile writes a definition back out as a SKILL.md, which is
// what the learning paths use to build a folder they are about to save.
func RenderDescriptionFile(definition Definition) []byte {
	lines := []string{"# " + definition.Name, "", definition.Description, "", "## Triggers", ""}
	for _, trigger := range definition.Triggers {
		lines = append(lines, "- "+trigger)
	}
	lines = append(lines, "", "## Permissions", "")
	if definition.Permissions.BrowserProfile != "" {
		lines = append(lines, "- "+browserProfileKey+": "+definition.Permissions.BrowserProfile)
	}
	for _, site := range definition.Permissions.Sites {
		lines = append(lines, "- "+siteKey+": "+site)
	}
	limit := definition.Permissions.DailyLimit
	if limit == 0 {
		limit = DefaultDailyLimit
	}
	lines = append(lines, fmt.Sprintf("- %s: %d", dailyLimitKey, limit))
	for _, number := range definition.Permissions.IrreversibleSteps {
		lines = append(lines, fmt.Sprintf("- %s: %d", irreversibleStepKey, number))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

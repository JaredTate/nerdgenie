// The split between a forgiving listing and a strict load is OpenCode's skill
// loader at ~/Code/opencode/packages/opencode/src/skill/index.ts, where a folder
// that will not parse is logged and passed over rather than allowed to break
// the session. Coeus keeps that for the listing, which rides in every prompt,
// and refuses a broken folder by name when a skill is actually used.

package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Folder is one skill folder, read and checked: what SKILL.md says, the steps,
// the dry run, and the body the model reads when the skill is used.
type Folder struct {
	// Definition is what SKILL.md says.
	Definition Definition
	// Steps are the numbered steps, and are empty on a skill that carries a
	// script instead.
	Steps []Step
	// Plan is the dry run test.md describes.
	Plan DryRunPlan
	// HasScript says the procedure is an executable rather than a step list.
	HasScript bool
	// Body is what Load hands back: SKILL.md and the procedure together.
	Body string
	// Path is the folder on disk, and is empty for a folder built in memory
	// that has not been saved yet.
	Path string
}

// ParseFolder checks a skill folder held in memory and returns what it says. It
// refuses a folder with a file missing, naming the file, because a skill about
// to be used has to be whole.
func ParseFolder(files map[string][]byte) (Folder, error) {
	description, held := files[DescriptionFile]
	if !held {
		return Folder{}, fmt.Errorf("this skill folder has no %s in it, so add one naming the skill and saying what it does", DescriptionFile)
	}
	definition, err := ParseDescriptionFile(description)
	if err != nil {
		return Folder{}, err
	}

	folder := Folder{Definition: definition}
	if err := readProcedure(&folder, files); err != nil {
		return Folder{}, err
	}
	test, held := files[TestFile]
	if !held {
		return Folder{}, fmt.Errorf("the skill %q has no %s in it, so add one saying how to dry run it", definition.Name, TestFile)
	}
	if folder.Plan, err = ParseTestFile(test); err != nil {
		return Folder{}, err
	}
	if _, held := files[ChangelogFile]; !held {
		return Folder{}, fmt.Errorf("the skill %q has no %s in it, so add one recording how it came to be", definition.Name, ChangelogFile)
	}
	if err := checkIrreversibleSteps(folder); err != nil {
		return Folder{}, err
	}
	folder.Body = bodyOf(description, files)
	return folder, nil
}

// readProcedure reads whichever of the two procedures the folder carries.
func readProcedure(folder *Folder, files map[string][]byte) error {
	steps, hasSteps := files[StepsFile]
	script, hasScript := files[ScriptFile]
	switch {
	case hasSteps && hasScript:
		return fmt.Errorf("the skill %q has both a %s and a %s, so keep the one that holds the real procedure", folder.Definition.Name, StepsFile, ScriptFile)
	case hasScript:
		if len(script) > MaxFileBytes {
			return fmt.Errorf("the %s of the skill %q is %d bytes and the most allowed is %d, so shorten it", ScriptFile, folder.Definition.Name, len(script), MaxFileBytes)
		}
		folder.HasScript = true
		return nil
	case hasSteps:
		parsed, err := ParseSteps(steps)
		if err != nil {
			return err
		}
		folder.Steps = parsed
		return nil
	default:
		return fmt.Errorf("the skill %q has neither a %s nor a %s, so add one of them to say what it does", folder.Definition.Name, StepsFile, ScriptFile)
	}
}

// checkIrreversibleSteps refuses a permissions block that marks a step the step
// list does not have, because a mark on a step that is not there protects
// nothing.
func checkIrreversibleSteps(folder Folder) error {
	if folder.HasScript {
		return nil
	}
	for _, number := range folder.Definition.Permissions.IrreversibleSteps {
		if number > len(folder.Steps) {
			return fmt.Errorf("the skill %q marks step %d as one that cannot be undone but %s has only %d steps, so correct the number", folder.Definition.Name, number, StepsFile, len(folder.Steps))
		}
	}
	return nil
}

// bodyOf builds what the model reads when the skill is used: the description
// file and the procedure, one after the other.
func bodyOf(description []byte, files map[string][]byte) string {
	parts := []string{strings.TrimRight(string(description), "\n")}
	if steps, held := files[StepsFile]; held && len(steps) > 0 {
		parts = append(parts, "## Steps", strings.TrimRight(string(steps), "\n"))
	}
	if _, held := files[ScriptFile]; held {
		parts = append(parts, "## Steps", "This skill runs the executable named "+ScriptFile+" in its own folder.")
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// ReadFolder reads one skill folder off disk and checks it. The name in
// SKILL.md has to be the folder's own name, so that a folder somebody renamed
// says so rather than answering to two names.
func ReadFolder(path string) (Folder, error) {
	files := map[string][]byte{}
	for _, name := range []string{DescriptionFile, StepsFile, ScriptFile, TestFile, ChangelogFile} {
		content, err := readSkillFile(filepath.Join(path, name))
		if err != nil {
			return Folder{}, err
		}
		if content != nil {
			files[name] = content
		}
	}

	folder, err := ParseFolder(files)
	if err != nil {
		return Folder{}, err
	}
	if wanted := filepath.Base(path); folder.Definition.Name != wanted {
		return Folder{}, fmt.Errorf("the folder %s holds a %s naming the skill %q, so make the heading and the folder name agree", wanted, DescriptionFile, folder.Definition.Name)
	}
	folder.Path = path
	return folder, nil
}

// readSkillFile reads one file of a skill folder, returning nothing at all when
// the file is not there and refusing one bigger than the cap by name rather
// than holding it in memory whole.
func readSkillFile(path string) ([]byte, error) {
	about, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot look at %s, so check that the skill folder can be read: %w", path, err)
	}
	if about.IsDir() {
		return nil, nil
	}
	if about.Size() > MaxFileBytes {
		return nil, fmt.Errorf("%s is %d bytes and the most allowed is %d, so shorten it", path, about.Size(), MaxFileBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s, so check that the skill folder can be read: %w", path, err)
	}
	return content, nil
}

// ReadSummary reads only the head of a skill folder: the name and the one line
// that rides in the prompt. It is what the listing and the trigger matcher use,
// so that neither of them ever fails because of a folder nobody is using.
func ReadSummary(path string) (Definition, error) {
	content, err := readSkillFile(filepath.Join(path, DescriptionFile))
	if err != nil {
		return Definition{}, err
	}
	if content == nil {
		return Definition{}, fmt.Errorf("the folder %s has no %s in it, so it is not a skill", path, DescriptionFile)
	}
	definition, err := ParseDescriptionFile(content)
	if err != nil {
		return Definition{}, err
	}
	if wanted := filepath.Base(path); definition.Name != wanted {
		return Definition{}, fmt.Errorf("the folder %s holds a %s naming the skill %q, so make the heading and the folder name agree", wanted, DescriptionFile, definition.Name)
	}
	return definition, nil
}

// CheckFileNames refuses a set of files that would be written anywhere but
// inside the skill's own folder, because the names come from outside the
// program and a name holding a separator is a way out of the folder.
func CheckFileNames(files map[string][]byte) error {
	for name := range files {
		if name == "" || name != filepath.Base(name) || name == "." || name == ".." || strings.HasPrefix(name, ".") {
			return fmt.Errorf("a skill folder cannot hold a file named %q, so use a plain file name such as %s", name, StepsFile)
		}
		if len(files[name]) > MaxFileBytes {
			return fmt.Errorf("the file %q is %d bytes and the most allowed in a skill folder is %d, so shorten it", name, len(files[name]), MaxFileBytes)
		}
	}
	return nil
}

// isIrreversible says whether the permissions block marks this step as one that
// cannot be undone.
func (folder Folder) isIrreversible(number int) bool {
	return slices.Contains(folder.Definition.Permissions.IrreversibleSteps, number)
}

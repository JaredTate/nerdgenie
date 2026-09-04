package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ApprovedByPersonFile is the store's own record, kept beside a skill, that a
// person ran that skill themselves and said yes to a step. A skill the model
// wrote holds no standing approval until this file is there, and the file is
// written by the store alone: a folder handed in holding a file of this name is
// refused, because a model that could write it could write itself the very yes
// it is waiting for.
const ApprovedByPersonFile = "approved-by-person"

// sourceLine is the line the store writes into the permissions block of a skill
// the model saved. It is a line the person reading the folder can see, and the
// same line the reader turns back into a source.
const sourceLine = "- " + sourceKey + ": " + string(contract.SkillSavedByModel)

// withSourceMark returns the SKILL.md with the store's own line about who saved
// it: one line in the permissions block saying the model did, and no line at all
// when a person did. Whatever the file said about its own source is taken out
// first, because the file comes from whoever is saving and the store's word is
// the one that counts.
func withSourceMark(content []byte, source contract.SkillSource) []byte {
	lines := withoutTheSourceLine(strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n"))
	if source != contract.SkillSavedByModel {
		return []byte(tidyEnd(lines))
	}
	at, found := whereTheBulletsOfThePermissionsBlockBegin(lines)
	if !found {
		return []byte(tidyEnd(append(lines, "", "## Permissions", "", sourceLine)))
	}
	return []byte(tidyEnd(slices.Insert(lines, at, sourceLine)))
}

// withoutTheSourceLine returns the lines with any source bullet taken out.
func withoutTheSourceLine(lines []string) []string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if key, _, separated := strings.Cut(bulletText(strings.TrimSpace(line)), ":"); separated && strings.ToLower(strings.TrimSpace(key)) == sourceKey {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

// whereTheBulletsOfThePermissionsBlockBegin is the line the store's own bullet
// belongs on: the first line after the permissions heading and the blank line
// under it. It says so when the file has no permissions block at all.
func whereTheBulletsOfThePermissionsBlockBegin(lines []string) (int, bool) {
	for number, line := range lines {
		if strings.ToLower(strings.TrimSpace(line)) != "## permissions" {
			continue
		}
		at := number + 1
		if at < len(lines) && strings.TrimSpace(lines[at]) == "" {
			at++
		}
		return at, true
	}
	return 0, false
}

// tidyEnd joins the lines back into a file that ends in one newline, so that
// taking a line out or putting one in does not change how the file ends.
func tidyEnd(lines []string) string {
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// aPersonHasApprovedIt says whether a person has run this skill themselves and
// said yes to a step of it.
func (store *Store) aPersonHasApprovedIt(name string) bool {
	_, err := os.Stat(filepath.Join(store.home.SkillFolder(name), ApprovedByPersonFile))
	return err == nil
}

// rememberThePersonSaidYes writes the store's record that a person ran this
// skill and approved a step of it, which is what a skill the model wrote waits
// for before its permissions block means anything. A record that cannot be
// written is not a reason to stop the run, because the run itself had its yes;
// the skill simply waits for the next one.
func (store *Store) rememberThePersonSaidYes(name string) {
	path := filepath.Join(store.home.SkillFolder(name), ApprovedByPersonFile)
	if store.aPersonHasApprovedIt(name) {
		return
	}
	line := fmt.Sprintf("A person ran this skill and said yes on %s.\n", store.clock.Now().UTC().Format(changelogDateLayout))
	_ = os.WriteFile(path, []byte(line), contract.DataFileMode)
}

// forgetThePersonSaidYes takes the record away, which every save does, because
// the person said yes to the procedure that was there and a save writes a new
// one.
func (store *Store) forgetThePersonSaidYes(name string) error {
	path := filepath.Join(store.home.SkillFolder(name), ApprovedByPersonFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot take away the record that a person approved the skill %q, so check that the home folder can be written: %w", name, err)
	}
	return nil
}

// waitingForAPersonToRunIt says whether this skill's permissions block means
// nothing yet: the model wrote it, and no person has run it and said yes.
func (store *Store) waitingForAPersonToRunIt(folder Folder) bool {
	return folder.Definition.Source == contract.SkillSavedByModel && !store.aPersonHasApprovedIt(folder.Definition.Name)
}

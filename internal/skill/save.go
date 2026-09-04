// The changelog and its rollback are Hermes' skill ledger written fresh for
// Coeus, at ~/Code/hermes-agent/tools/skill_ledger.py: every change is appended
// rather than edited, the copy a change replaced is kept beside the skill, and a
// rollback first keeps what it is about to overwrite, so that the rollback is
// itself undoable. Coeus keeps whole files in numbered folders rather than
// content-addressed blobs, because a person reading ~/.nerdgenie by hand should be
// able to see what the old version said.

package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// changelogDateLayout is how a changelog entry writes its date: universal time
// to the second, so that entries sort the way they happened wherever the
// machine is.
const changelogDateLayout = time.RFC3339

// Save writes a skill folder, keeping the copy it replaced beside it and
// recording the change in the changelog. A folder that gives only a SKILL.md is
// completed with the other three files, so that everything the store saves can
// afterwards be loaded and run.
func (store *Store) Save(_ context.Context, source contract.SkillSource, name string, files map[string][]byte) error {
	if !contract.KnownSkillSource(source) {
		return fmt.Errorf("the skill %q is being saved by %q, and a save says whether the person or the model is saving, so pass one of those two", name, source)
	}
	if err := CheckName(name); err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("the skill %q has no files, so a skill folder needs at least a %s in it", name, DescriptionFile)
	}
	if err := CheckFileNames(files); err != nil {
		return err
	}

	whole := completeFolder(name, files, store.changelogOnDisk(name))
	whole[DescriptionFile] = withSourceMark(whole[DescriptionFile], source)
	folder, err := ParseFolder(whole)
	if err != nil {
		return err
	}
	if folder.Definition.Name != name {
		return fmt.Errorf("the skill is being saved as %q but its %s names it %q, so make the heading and the folder name agree", name, DescriptionFile, folder.Definition.Name)
	}
	if err := store.roomForAnotherSkill(name); err != nil {
		return err
	}

	kept, err := store.keepWhatIsThere(name)
	if err != nil {
		return err
	}
	if err := store.writeFolder(name, whole); err != nil {
		return err
	}
	if err := store.forgetThePersonSaidYes(name); err != nil {
		return err
	}
	return store.recordChange(name, savedEntry(kept))
}

// savedEntry is the changelog line one save writes, which says how to undo it.
func savedEntry(kept int) string {
	if kept == 0 {
		return "saved for the first time. To undo: /skills remove"
	}
	return fmt.Sprintf("saved, keeping the copy it replaced as version %d. To undo: /skills rollback", kept)
}

// completeFolder fills in the files a skill folder cannot do without, so that a
// caller who wrote only a SKILL.md still ends up with a folder that loads. The
// changelog a save does not carry is the one already on disk, because a save
// adds to the record of a skill rather than starting it again.
func completeFolder(name string, files map[string][]byte, changelog []byte) map[string][]byte {
	whole := map[string][]byte{}
	for key, content := range files {
		whole[key] = content
	}
	_, hasSteps := whole[StepsFile]
	_, hasScript := whole[ScriptFile]
	if !hasSteps && !hasScript {
		whole[StepsFile] = []byte{}
	}
	if _, held := whole[TestFile]; !held {
		whole[TestFile] = RenderTestFile(name, DryRunPlan{})
	}
	if _, held := whole[ChangelogFile]; !held {
		whole[ChangelogFile] = changelog
	}
	if len(whole[ChangelogFile]) == 0 {
		whole[ChangelogFile] = []byte("# changelog for " + name + "\n\n")
	}
	return whole
}

// changelogOnDisk is what a skill's changelog already says, or nothing at all
// when the skill is new.
func (store *Store) changelogOnDisk(name string) []byte {
	content, err := os.ReadFile(filepath.Join(store.home.SkillFolder(name), ChangelogFile))
	if err != nil {
		return nil
	}
	return content
}

// roomForAnotherSkill refuses a new skill once the machine holds as many as it
// keeps, because a folder of skills nobody can read through is a folder nobody
// uses.
func (store *Store) roomForAnotherSkill(name string) error {
	if store.folderExists(name) {
		return nil
	}
	names, err := store.skillNames()
	if err != nil {
		return err
	}
	if len(names) >= MaxSkills {
		return fmt.Errorf("this machine already holds %d skills, which is all it keeps, so remove one before saving %q", MaxSkills, name)
	}
	return nil
}

// keepWhatIsThere copies the skill's current files into the next numbered
// version folder and returns that number, or zero when the skill is new. The
// changelog itself is left out, because it is the running record and a restore
// must never take it back to what it said before.
func (store *Store) keepWhatIsThere(name string) (int, error) {
	if !store.folderExists(name) {
		return 0, nil
	}
	numbers, err := store.versionNumbers(name)
	if err != nil {
		return 0, err
	}
	next := 1
	if len(numbers) > 0 {
		next = numbers[len(numbers)-1] + 1
	}

	into := filepath.Join(store.versionsFolder(name), strconv.Itoa(next))
	if err := os.MkdirAll(into, contract.HomeFolderMode); err != nil {
		return 0, fmt.Errorf("cannot make the folder %s to keep the copy being replaced, so check that the home folder can be written: %w", into, err)
	}
	for _, file := range procedureFiles() {
		if err := copyOneFile(filepath.Join(store.home.SkillFolder(name), file), filepath.Join(into, file)); err != nil {
			return 0, err
		}
	}
	return next, store.forgetOldestVersions(name, append(numbers, next))
}

// procedureFiles are the files a version keeps, which is every file of a skill
// folder except the changelog.
func procedureFiles() []string {
	return []string{DescriptionFile, StepsFile, ScriptFile, TestFile}
}

// forgetOldestVersions removes the oldest kept copies once there are more than
// the store keeps, because a skill edited every day would otherwise grow
// without end.
func (store *Store) forgetOldestVersions(name string, numbers []int) error {
	for len(numbers) > MaxVersions {
		path := filepath.Join(store.versionsFolder(name), strconv.Itoa(numbers[0]))
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("cannot remove the old version at %s, so check that the home folder can be written: %w", path, err)
		}
		numbers = numbers[1:]
	}
	return nil
}

// versionNumbers returns the numbers of the copies kept beside a skill, in
// order.
func (store *Store) versionNumbers(name string) ([]int, error) {
	entries, err := os.ReadDir(store.versionsFolder(name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the versions folder of the skill %q, so check that the home folder can be read: %w", name, err)
	}
	numbers := []int{}
	for _, entry := range entries {
		number, err := strconv.Atoi(entry.Name())
		if entry.IsDir() && err == nil && number > 0 {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	return numbers, nil
}

// writeFolder writes a whole skill folder, taking out any of the known files
// the new copy does not have, so that a skill that swapped its steps for a
// script does not keep both.
func (store *Store) writeFolder(name string, files map[string][]byte) error {
	path := store.home.SkillFolder(name)
	if err := os.MkdirAll(path, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("cannot make the folder %s for the skill, so check that the home folder can be written: %w", path, err)
	}
	for _, file := range procedureFiles() {
		if _, held := files[file]; held {
			continue
		}
		if err := os.Remove(filepath.Join(path, file)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove the old %s of the skill %q, so check that the home folder can be written: %w", file, name, err)
		}
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(path, file), content, modeFor(file)); err != nil {
			return fmt.Errorf("cannot write the %s of the skill %q, so check that the home folder can be written: %w", file, name, err)
		}
	}
	return nil
}

// modeFor is the mode one file of a skill folder is written with. The script is
// the one file that has to be runnable.
func modeFor(file string) os.FileMode {
	if file == ScriptFile {
		return contract.HomeFolderMode
	}
	return contract.DataFileMode
}

// copyOneFile copies a file if it is there and does nothing if it is not, which
// is what keeping a version of a skill that has no script needs.
func copyOneFile(from string, to string) error {
	content, err := os.ReadFile(from)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read %s while keeping the copy being replaced, so check that it can be read: %w", from, err)
	}
	if err := os.WriteFile(to, content, modeFor(filepath.Base(from))); err != nil {
		return fmt.Errorf("cannot write %s while keeping the copy being replaced, so check that the home folder can be written: %w", to, err)
	}
	return nil
}

// recordChange appends one line to a skill's changelog, saying when the change
// happened and how to undo it.
func (store *Store) recordChange(name string, what string) error {
	path := filepath.Join(store.home.SkillFolder(name), ChangelogFile)
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read the %s of the skill %q, so check that the home folder can be read: %w", ChangelogFile, name, err)
	}
	if len(content) == 0 {
		content = []byte("# changelog for " + name + "\n\n")
	}
	if !strings.HasSuffix(string(content), "\n") {
		content = append(content, '\n')
	}

	line := fmt.Sprintf("- %s %s %s\n", store.clock.Now().UTC().Format(changelogDateLayout), what, name)
	if err := os.WriteFile(path, append(content, line...), contract.DataFileMode); err != nil {
		return fmt.Errorf("cannot write the %s of the skill %q, so check that the home folder can be written: %w", ChangelogFile, name, err)
	}
	return nil
}

// Rollback puts a skill back to the copy kept beside it by the newest save,
// keeping what is there now as a version of its own first, so that the rollback
// can itself be rolled back.
func (store *Store) Rollback(_ context.Context, name string) (string, error) {
	if _, err := store.readOrName(name); err != nil {
		return "", err
	}
	numbers, err := store.versionNumbers(name)
	if err != nil {
		return "", err
	}
	if len(numbers) == 0 {
		return "", fmt.Errorf("the skill %q has no earlier copy kept beside it, so there is nothing to roll back to", name)
	}

	restoring := numbers[len(numbers)-1]
	kept, err := store.keepWhatIsThere(name)
	if err != nil {
		return "", err
	}
	if err := store.restoreVersion(name, restoring); err != nil {
		return "", err
	}
	what := fmt.Sprintf("rolled back to version %d, keeping what was there as version %d. To undo: /skills rollback", restoring, kept)
	if err := store.recordChange(name, what); err != nil {
		return "", err
	}
	return fmt.Sprintf("The skill %q is back to version %d, and what was there is kept as version %d.", name, restoring, kept), nil
}

// readOrName says whether a skill folder is there at all, so that a rollback of
// a name nobody has says what is missing.
func (store *Store) readOrName(name string) (string, error) {
	if err := CheckName(name); err != nil {
		return "", err
	}
	if !store.folderExists(name) {
		return "", fmt.Errorf("there is no skill named %q, so run /skills to see what there is", name)
	}
	return name, nil
}

// restoreVersion copies one kept version back over the skill's own files.
func (store *Store) restoreVersion(name string, number int) error {
	from := filepath.Join(store.versionsFolder(name), strconv.Itoa(number))
	files := map[string][]byte{}
	for _, file := range procedureFiles() {
		content, err := os.ReadFile(filepath.Join(from, file))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot read the kept %s of the skill %q, so check that the home folder can be read: %w", file, name, err)
		}
		files[file] = content
	}
	if len(files) == 0 {
		return fmt.Errorf("version %d of the skill %q holds no files, so there is nothing in it to go back to", number, name)
	}
	return store.writeFolder(name, files)
}

// Remove takes a skill out of use without throwing it away: the folder is
// renamed so that nothing lists it any more and a person can still read it.
func (store *Store) Remove(_ context.Context, name string) (string, error) {
	if _, err := store.readOrName(name); err != nil {
		return "", err
	}
	from := store.home.SkillFolder(name)
	for attempt := range MaxVersions {
		to := removedName(store.home.SkillFolder(name), attempt)
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return "", fmt.Errorf("cannot rename the folder of the skill %q, so check that the home folder can be written: %w", name, err)
		}
		return fmt.Sprintf("The skill %q is removed. Its folder is kept at %s.", name, to), nil
	}
	return "", fmt.Errorf("the skill %q has been removed and put back so many times that there is no free name left, so clear out the removed folders", name)
}

// removedName is what a removed skill's folder is called, which is a name no
// skill may have, so that the listing passes over it.
func removedName(path string, attempt int) string {
	if attempt == 0 {
		return path + ".removed"
	}
	return fmt.Sprintf("%s.removed-%d", path, attempt+1)
}

// Changelog returns what a skill's changelog says, which is what "/skills show"
// prints under the skill.
func (store *Store) Changelog(name string) (string, error) {
	if _, err := store.readOrName(name); err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(store.home.SkillFolder(name), ChangelogFile))
	if os.IsNotExist(err) {
		return "", errors.New("this skill has no changelog yet, so save it once to start one")
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the %s of the skill %q, so check that the home folder can be read: %w", ChangelogFile, name, err)
	}
	return string(content), nil
}

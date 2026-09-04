package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

// AskFunc shows the user what is about to happen and waits for the answer. It
// is the one function cmd/coeus/serve.go wires to the channel the user is on,
// and it carries both the offers to save a new skill and the yes a step that
// cannot be undone needs.
type AskFunc func(ctx context.Context, preview contract.Preview) (contract.PreviewAnswerWithReason, error)

// StandingApprover is the part of the permission function a skill needs: a
// place to register the standing approvals its permissions block grants. The
// real one is *permission.Decider.
type StandingApprover interface {
	// RegisterStandingApproval gives the permission function one standing
	// approval, and returns an error naming what is missing when it cannot be
	// used.
	RegisterStandingApproval(approval permission.StandingApproval) error
}

// Options is what a skill store is built from.
type Options struct {
	// Home is the agent's home folder, whose skills folder holds the skills.
	Home contract.Home
	// Clock is where the changelog's dates and a standing approval's expiry are
	// read from.
	Clock contract.Clock
	// Tools is the registry a replayed step calls through.
	Tools contract.ToolRegistry
	// Permission is the rulebook every replayed step is put through.
	Permission contract.Permission
	// Standing is where a skill's permissions block becomes standing
	// approvals, and may be left out when nothing registers them.
	Standing StandingApprover
	// Ask shows the user a preview and waits, and may be left out, in which
	// case a step needing a yes stops and reports instead.
	Ask AskFunc
}

// Store is the skills folder on disk: one folder per skill, read on demand.
type Store struct {
	home       contract.Home
	clock      contract.Clock
	tools      contract.ToolRegistry
	permission contract.Permission
	standing   StandingApprover
	ask        AskFunc

	guard sync.Mutex
	// registered says, for each skill, the day its standing approvals were
	// registered, so that a skill run twice in a day does not get its daily
	// limit twice.
	registered map[string]string
}

// New builds a skill store over the skills folder in the agent's home. It
// refuses to build one without a home, a clock, a tool registry, and a
// permission function, because a skill that ran without any of those would be
// running outside the harness.
func New(options Options) (*Store, error) {
	if options.Home.Root == "" {
		return nil, errors.New("the skill store was built without a home folder, so pass the layout the skills folder lives in")
	}
	if options.Clock == nil {
		return nil, errors.New("the skill store was built without a clock, so pass one, because a changelog entry carries a date")
	}
	if options.Tools == nil {
		return nil, errors.New("the skill store was built without a tool registry, so pass one, because a skill replays through the tools")
	}
	if options.Permission == nil {
		return nil, errors.New("the skill store was built without a permission function, so pass one, because every step is ruled on")
	}
	return &Store{
		home:       options.Home,
		clock:      options.Clock,
		tools:      options.Tools,
		permission: options.Permission,
		standing:   options.Standing,
		ask:        options.Ask,
		registered: map[string]string{},
	}, nil
}

// List returns every skill's name and one-liner for the prompt, in name order.
// A folder it cannot read is passed over rather than reported, because the
// prompt is built on every turn and one broken folder must not empty it.
func (store *Store) List(_ context.Context) ([]contract.SkillSummary, error) {
	definitions, err := store.readSummaries()
	if err != nil {
		return nil, err
	}
	listed := make([]contract.SkillSummary, 0, len(definitions))
	for _, definition := range definitions {
		listed = append(listed, definition.Summary())
	}
	return listed, nil
}

// readSummaries reads the head of every skill folder, in name order, passing
// over anything that is not a skill.
func (store *Store) readSummaries() ([]Definition, error) {
	entries, err := os.ReadDir(store.home.SkillsFolder())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the skills folder %s, so check that it exists and can be read: %w", store.home.SkillsFolder(), err)
	}

	definitions := []Definition{}
	for _, entry := range entries {
		if !entry.IsDir() || CheckName(entry.Name()) != nil {
			continue
		}
		definition, err := ReadSummary(store.home.SkillFolder(entry.Name()))
		if err != nil {
			continue
		}
		definitions = append(definitions, definition)
		if len(definitions) >= MaxSkills {
			break
		}
	}
	sort.Slice(definitions, func(first int, second int) bool {
		return definitions[first].Name < definitions[second].Name
	})
	return definitions, nil
}

// Load returns one skill's body: what SKILL.md says and the procedure under it.
// This is the strict path, so a folder with a file missing is refused with the
// file named.
func (store *Store) Load(_ context.Context, name string) (string, error) {
	folder, err := store.read(name)
	if err != nil {
		return "", err
	}
	return folder.Body, nil
}

// read finds one skill folder and checks it whole.
func (store *Store) read(name string) (Folder, error) {
	if err := CheckName(name); err != nil {
		return Folder{}, err
	}
	path := store.home.SkillFolder(name)
	if about, err := os.Stat(path); err != nil || !about.IsDir() {
		return Folder{}, fmt.Errorf("there is no skill named %q, so run /skills to see what there is", name)
	}
	return ReadFolder(path)
}

// Match says which skill's trigger words the message holds. A skill fires only
// when the message holds every one of its triggers, and a message that fires
// two skills fires neither, because guessing between two procedures is worse
// than handing the message to the model.
func (store *Store) Match(_ context.Context, text string) (contract.SkillMatch, error) {
	definitions, err := store.readSummaries()
	if err != nil {
		return contract.SkillMatch{}, err
	}

	words := strings.Fields(strings.ToLower(text))
	matched := Definition{}
	for _, definition := range definitions {
		if !triggersFire(definition.Triggers, words) {
			continue
		}
		if matched.Name != "" {
			return contract.SkillMatch{}, nil
		}
		matched = definition
	}
	if matched.Name == "" {
		return contract.SkillMatch{}, nil
	}
	// The match carries the skill's own budget, so that the task the message
	// starts can be given it: design section 3, rule 3.
	return contract.SkillMatch{Name: matched.Name, Matched: true, Rounds: matched.Rounds, Time: matched.Time}, nil
}

// triggersFire says whether the message holds every trigger the skill names. A
// skill with no triggers never fires from a message and is run by name.
func triggersFire(triggers []string, words []string) bool {
	if len(triggers) == 0 {
		return false
	}
	for _, trigger := range triggers {
		if !holdsPhrase(words, strings.Fields(trigger)) {
			return false
		}
	}
	return true
}

// holdsPhrase says whether the message's words hold the trigger's words in a
// row, which is how a one-word trigger and a several-word trigger are matched
// by the same rule.
func holdsPhrase(words []string, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(words) {
		return false
	}
	for start := 0; start+len(phrase) <= len(words); start++ {
		found := true
		for offset, word := range phrase {
			if strings.Trim(words[start+offset], ".,!?;:\"'") != word {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

// skillNames returns the names of the folders in the skills folder, whether or
// not they read as skills, so that a name already taken is never written over.
func (store *Store) skillNames() ([]string, error) {
	entries, err := os.ReadDir(store.home.SkillsFolder())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the skills folder %s, so check that it exists and can be read: %w", store.home.SkillsFolder(), err)
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && CheckName(entry.Name()) == nil {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// folderExists says whether one skill's folder is already on disk.
func (store *Store) folderExists(name string) bool {
	about, err := os.Stat(store.home.SkillFolder(name))
	return err == nil && about.IsDir()
}

// versionsFolder is where the copies a skill's saves replaced are kept.
func (store *Store) versionsFolder(name string) string {
	return filepath.Join(store.home.SkillFolder(name), VersionsFolder)
}

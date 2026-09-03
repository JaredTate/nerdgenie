package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// checksFolderName is the folder under the home that a visual check writes its
// pictures into, one folder per walk. A second check of the same walk replaces
// the pictures of the first, so that checking a walk every day does not fill the
// disk with folders nobody named.
const checksFolderName = "checks"

// WalkOptions is what the /walk command is built from. The browser is the only
// one it cannot do anything without: a walk with no model heals nothing, a walk
// with no screen takes no step the recording never named, a walk with no skill
// store cannot be recorded, and a walk with no home has nowhere to put its
// pictures. Each of the four says so in plain words when it is asked for
// something it has not got, rather than being refused at the door.
type WalkOptions struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// Model is asked, once per replay, which element a failed step's intent
	// meant, and may be left out.
	Model contract.Model
	// Ask shows the user a preview and waits. It is used when the command was
	// typed somewhere that cannot be asked on, and the screen the command came
	// from is used when there is one.
	Ask skill.AskFunc
	// Skills is where a recorded walk and an approved patch are written.
	Skills SkillSaver
	// Home is where the skill folders are read from and where the pictures of a
	// visual check are written.
	Home contract.Home
}

// WalkCommand is the /walk command: record what the browser is showing as a
// walk, replay a walk with no model call, or walk one and photograph every step.
// The orchestrator registers this value in serve.go beside the skills command.
func WalkCommand(options WalkOptions) contract.Command {
	return contract.Command{
		Name: "walk",
		Help: "record, replay, or check a browser walk: /walk record|replay|check <name>",
		Run: func(ctx context.Context, arguments string, where contract.CommandContext) (string, error) {
			return options.run(ctx, arguments, where)
		},
	}
}

// run reads the words after "/walk" and does what they ask for.
func (options WalkOptions) run(ctx context.Context, arguments string, where contract.CommandContext) (string, error) {
	word, rest, _ := strings.Cut(strings.TrimSpace(arguments), " ")
	name := strings.TrimSpace(rest)
	switch word {
	case "":
		return theThreeForms, nil
	case "record", "replay", "check":
		if name == "" {
			return "", fmt.Errorf("/walk %s needs the name of a walk, such as /walk %s shop-checkout", word, word)
		}
	default:
		return "", fmt.Errorf("walk does not know the word %q, so use /walk record, /walk replay, or /walk check with a name", word)
	}
	if options.Browser == nil {
		return "", errors.New("the browser tools are switched off, so there is no browser to walk in; install Node and the browser worker bundle and start the agent again")
	}
	if options.Home.Root == "" {
		return "", errors.New("the /walk command has no home folder behind it, so there is nowhere to read a walk from and nowhere to put the pictures of a check")
	}
	if word == "record" {
		return options.record(ctx, name)
	}
	folder, err := options.readTheWalk(name)
	if err != nil {
		return "", err
	}
	if word == "check" {
		return options.check(ctx, folder)
	}
	return options.replay(ctx, folder, where)
}

// theThreeForms is what /walk on its own answers, because the three things it
// does are worth naming in the terminal rather than only in the help listing.
const theThreeForms = `A walk is a browser procedure saved as a skill. There are three things to do with one:
  /walk record <name>  writes the page the browser is on down as the first step of a new walk
  /walk replay <name>  walks a saved recording again, with no model call unless a step has moved
  /walk check <name>   walks it and photographs every step, saying what each one showed`

// readTheWalk loads one skill folder off disk and refuses a skill that is not a
// browser recording, naming the skill either way.
func (options WalkOptions) readTheWalk(name string) (skill.Folder, error) {
	folder, err := skill.ReadFolder(options.Home.SkillFolder(name))
	if err != nil {
		return skill.Folder{}, fmt.Errorf("cannot read the walk %q, so check the name against /skills: %w", name, err)
	}
	if _, err := StepsOf(folder); err != nil {
		return skill.Folder{}, fmt.Errorf("the skill %q is not a browser walk: %w", name, err)
	}
	return folder, nil
}

// record writes the page the browser is on down as the first step of a new walk.
// It is the first step and not the whole walk because nothing in the browser
// worker reports a person's own clicks: the harness can write down what it does
// itself, and a person adds the rest of the steps by hand or lets the agent take
// them in a task, where they are recorded as they happen.
func (options WalkOptions) record(ctx context.Context, name string) (string, error) {
	if options.Skills == nil {
		return "", errors.New("the /walk command has no skill store behind it, so there is nowhere to save the walk")
	}
	page, err := options.Browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return "", fmt.Errorf("there is no page open in the browser to record, so open one first: %w", err)
	}
	recorder, err := NewRecorder(options.Browser)
	if err != nil {
		return "", err
	}
	expectation := "the page " + page.Title + " is on the screen"
	if _, err := recorder.Open(ctx, "Open "+page.URL+".", page.URL, expectation); err != nil {
		return "", err
	}
	if err := recorder.Save(ctx, contract.SkillSavedByPerson, options.Skills, definitionOfARecordedWalk(name, page)); err != nil {
		return "", err
	}
	return fmt.Sprintf("I wrote the page %s down as step 1 of the walk %q. I cannot see your own clicks in the browser, "+
		"so add the rest of the steps to %s, or let the agent take them in a task, where they are recorded as they happen. "+
		"Then /walk replay %s walks it again and /walk check %s photographs every step.",
		page.URL, name, filepath.Join(options.Home.SkillFolder(name), skill.StepsFile), name, name), nil
}

// definitionOfARecordedWalk is the SKILL.md a recorded walk starts life with: it
// may visit the one site it was recorded on and nothing else, it fires from no
// message because a procedure nobody has finished writing should not fire on its
// own, and it marks no step as one that cannot be undone.
func definitionOfARecordedWalk(name string, page contract.Snapshot) skill.Definition {
	description := "Walks " + siteOf(page.URL) + ", recorded from the page the browser was on."
	if runes := []rune(description); len(runes) > skill.MaxDescriptionRunes {
		description = string(runes[:skill.MaxDescriptionRunes])
	}
	return skill.Definition{
		Name:        name,
		Description: description,
		Permissions: skill.Permissions{
			Sites:      sitesOf(page.URL),
			DailyLimit: skill.DefaultDailyLimit,
		},
	}
}

// siteOf is the host of an address, in plain words for a description.
func siteOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return address
	}
	return parsed.Hostname()
}

// sitesOf is the one website a recorded walk may visit, and no list at all when
// the address has no host to name, because a permissions block naming nothing is
// safer than one naming something the agent made up.
func sitesOf(address string) []string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return nil
	}
	return []string{parsed.Hostname()}
}

// replay walks a saved recording again. The person who typed the command is the
// one asked about a step the recording never named, and the report is the
// answer whether or not every step did what it was recorded to do.
func (options WalkOptions) replay(ctx context.Context, folder skill.Folder, where contract.CommandContext) (string, error) {
	replayer, err := New(Options{
		Browser: options.Browser,
		Model:   options.Model,
		Ask:     options.askOn(where),
		Skills:  options.Skills,
	})
	if err != nil {
		return "", err
	}
	report, err := replayer.Replay(ctx, folder)
	if err != nil {
		return "", err
	}
	return report.String(), nil
}

// check walks the recording and photographs every step into the home's own
// checks folder, one folder per walk.
func (options WalkOptions) check(ctx context.Context, folder skill.Folder) (string, error) {
	checker, err := NewChecker(options.Browser)
	if err != nil {
		return "", err
	}
	into := filepath.Join(options.Home.Root, checksFolderName, folder.Definition.Name)
	result, err := checker.CheckFolder(ctx, folder, "", into)
	if err != nil {
		return "", err
	}
	return result.Markdown(), nil
}

// askOn is the screen a preview goes to: the one the command was typed on when
// there is one, so that the person who asked for the walk is the person asked
// about it, and the one the command was built with otherwise.
func (options WalkOptions) askOn(where contract.CommandContext) skill.AskFunc {
	if where.Channel != nil {
		return where.Channel.ShowPreview
	}
	return options.Ask
}

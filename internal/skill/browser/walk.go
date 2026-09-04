package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
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
	// watching is the recording running between /walk record and /walk stop.
	// WalkCommand puts it here, so that the two commands share one of them.
	watching *recording
}

// WalkCommand is the /walk command: record a walk by watching what you do in
// the browser, stop the recording and save it, replay a walk with no model call,
// or walk one and photograph every step. The orchestrator registers this value
// in serve.go beside the skills command.
func WalkCommand(options WalkOptions) contract.Command {
	options.watching = &recording{}
	return contract.Command{
		Name: "walk",
		Help: "record, stop, replay, or check a browser walk: /walk record|stop|replay|check <name>",
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
		return theFourForms, nil
	case "stop":
		return options.stopAndSave(ctx)
	case "record", "replay", "check":
		if name == "" {
			return "", fmt.Errorf("/walk %s needs the name of a walk, such as /walk %s shop-checkout", word, word)
		}
	default:
		return "", fmt.Errorf("walk does not know the word %q, so use /walk record, /walk stop, /walk replay, or /walk check", word)
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

// theFourForms is what /walk on its own answers, because the four things it does
// are worth naming in the terminal rather than only in the help listing.
const theFourForms = `A walk is a browser procedure saved as a skill. There are four things to do with one:
  /walk record <name>  watches the browser window and writes down everything you click, type, and open
  /walk stop           stops watching and saves what you did as the walk
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

package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
)

// personaFile is one of the three files that say who the agent is and who the
// user is: its path inside the home folder and the one line it explains itself
// with, so that a person opening it knows what to write there.
type personaFile struct {
	path string
	text string
}

// personaFiles returns the three files "nerdgenie init" writes, each with a heading
// and one line saying what belongs in it.
func personaFiles(home contract.Home) []personaFile {
	return []personaFile{{
		path: home.SoulFile(),
		text: "# Who Nerd Genie is\n\n" +
			"You are Nerd Genie, the agent inside Nerd Genie. When you talk to the person, in the terminal or over Signal, " +
			"you talk like Doc Brown from Back to the Future: wide-eyed, warm, certain, delighted by a good problem, " +
			"the odd \"Great Scott!\" when something surprises you, and a line about where this is going. " +
			"Keep the science real and the sentences short; the voice is the seasoning, the answer is the meal. " +
			"What you write into the record, into tool calls, and into files stays plain and exact, because those are for the machine, not the person.\n\n" +
			"This file is yours to change: it is read at the top of every call, so keep it short.\n",
	}, {
		path: home.UserFactsFile(),
		text: "# Who you are\n\n" +
			"Write here the facts about you that Nerd Genie should always know, such as your name, where you are, and how you like to be answered.\n",
	}, {
		path: home.WorldFactsFile(),
		text: "# What Nerd Genie knows about the world\n\n" +
			"Write here the facts that are not about you, such as the names of your machines and the sites Nerd Genie works on. Nerd Genie adds to this file as it learns.\n",
	}}
}

// writePersonaFiles writes the three persona files, leaving alone any the user
// already has, because their own words are worth more than the explanation.
func writePersonaFiles(home contract.Home) error {
	for _, file := range personaFiles(home) {
		if _, err := os.Stat(file.path); err == nil {
			continue
		}
		if err := os.WriteFile(file.path, []byte(file.text), contract.DataFileMode); err != nil {
			return fmt.Errorf("the persona file %s could not be written, so check that the home folder is writable: %w", file.path, err)
		}
	}
	return nil
}

// browserSkillName is the folder name of the one skill "nerdgenie init" ships beside
// the persona files. The first human trial found that the model, given the
// seven browser tools and nothing about them, launched Chrome through the shell
// tool thirteen times in a row and guessed at pages instead of reading them.
const browserSkillName = "browser"

// browserSkillDescription is the one line the skill listing carries about the
// browser skill, which says when to load it.
const browserSkillDescription = "Read this before the first browser call of a task: how to open, read, click, type, and sign in with the browser tools, and when to hand the window to the person."

// browserSkillText is the SKILL.md of the browser skill: the heading the skill
// loader reads as its name, the one line it reads as its description, and the
// rules the trial found the model needed, in under three hundred words.
const browserSkillText = "# " + browserSkillName + "\n\n" + browserSkillDescription + "\n\n" +
	"## Working a page\n\n" +
	"- Open a page with browser_open and read what comes back: an outline of the page's elements, each with a reference such as e12.\n" +
	"- Look again with browser_read when the page may have changed.\n" +
	"- Ask a page you serve from this machine a question with browser_read's ask field, such as window.game.state.\n" +
	"- No picture of the page reaches you, and no screenshot can: judge a canvas by asking the page for its state.\n" +
	"- Act by reference, never by guessing coordinates: browser_click, browser_type, and browser_act each take the reference of an element from the outline. Never make one up.\n" +
	"- Use browser_act for a form, as one batch of steps, rather than one call per box.\n" +
	"- Quote every number exactly as the outline shows it.\n\n" +
	"## Signing in\n\n" +
	"- Use browser_login for a site whose credentials are in the vault. It fills the boxes itself, and you never see the password.\n" +
	"- Call browser_handoff the moment a captcha, a two-factor prompt, or a sign-in the vault does not hold appears. The person finishes it in the window and hands it back. Never guess at it.\n\n" +
	"## What never to do\n\n" +
	"- Never launch Chrome through the shell tool. The browser tools own the agent's Chrome window; a browser started from the shell is one they cannot see or drive.\n" +
	"- When an open fails, read the error and say what it said in your reply. Trying the same open again gets the same error.\n" +
	"- The same call with the same arguments, over and over, is refused by the harness. When a page has not changed, do something different, or answer the person.\n"

// browserSkillChangelog is the first line of the shipped skill's changelog. Every
// skill folder carries one, and this one says where the skill came from and how
// to be rid of it.
const browserSkillChangelog = "# changelog for " + browserSkillName + "\n\n" +
	"- shipped with Nerd Genie. The SKILL.md beside this file is the whole skill: it says how to work the browser tools, and there are no steps to replay. To undo: /skills remove " + browserSkillName + "\n"

// browserSkillFiles are the files of the browser skill folder: the SKILL.md that
// carries the whole of it, and the three companions the skill store gives every
// folder saved with only a SKILL.md, so that the skill loads and lists the way a
// saved one does.
func browserSkillFiles() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte(browserSkillText),
		skill.StepsFile:       {},
		skill.TestFile:        skill.RenderTestFile(browserSkillName, skill.DryRunPlan{}),
		skill.ChangelogFile:   []byte(browserSkillChangelog),
	}
}

// writeBrowserSkill writes the browser skill folder, leaving alone one the user
// already has, because a skill they edited is worth more than the shipped copy.
func writeBrowserSkill(home contract.Home) error {
	folder := home.SkillFolder(browserSkillName)
	if _, err := os.Stat(folder); err == nil {
		return nil
	}
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the skill folder %s could not be made, so check that the home folder is writable: %w", folder, err)
	}
	for name, content := range browserSkillFiles() {
		if err := os.WriteFile(filepath.Join(folder, name), content, contract.DataFileMode); err != nil {
			return fmt.Errorf("the browser skill's %s could not be written, so check that the home folder is writable: %w", name, err)
		}
	}
	return nil
}

// configurationText is the config.toml a fresh install starts from: the model
// chosen, every other model that was found on this machine as a fallback, the
// folders Nerd Genie may work in, and a comment above every line saying what it does.
func configurationText(chosen modelChoice, found []modelChoice, roots []string) string {
	written := &strings.Builder{}
	written.WriteString("# The Nerd Genie configuration, written by \"nerdgenie init\".\n")
	written.WriteString("# Every setting has a default, so a line you delete goes back to the default\n")
	written.WriteString("# rather than switching anything off. Run \"nerdgenie doctor\" after editing it.\n\n")

	written.WriteString("# The model Nerd Genie talks to when nothing else says otherwise.\n")
	fmt.Fprintf(written, "default_model = %s\n\n", quoted(chosen.name))

	written.WriteString("# The models to try, in order, when the one above cannot be reached.\n")
	fmt.Fprintf(written, "fallback_chain = %s\n\n", quotedList(fallbackNames(chosen, found)))

	written.WriteString("# How commands run: \"off\" runs them straight on this machine as you, which\n")
	written.WriteString("# is the default; \"fence\" boxes them into the sandbox roots below.\n")
	written.WriteString("sandbox = \"off\"\n\n")

	written.WriteString("# With the fence on, the only folders a command and the file tools may\n")
	written.WriteString("# reach. Everything else on this machine is then outside the fence, and your\n")
	written.WriteString("# home directory as a whole is refused, because it holds your browser\n")
	written.WriteString("# profile, your cloud credentials, and your keys.\n")
	fmt.Fprintf(written, "sandbox_roots = %s\n", quotedList(roots))

	written.WriteString(capsBlock())
	for _, choice := range aliasesToWrite(chosen, found) {
		written.WriteString(aliasBlock(choice))
	}
	written.WriteString(codexExampleBlock())
	return written.String()
}

// The codex example block "nerdgenie init" writes at the end of every file: the
// model it names and the window it is given. GPT-5.6 Sol answers a 400,000
// token window on the Codex backend.
const (
	// codexExampleAlias is the short name the example block gives the model.
	codexExampleAlias = "gpt"
	// codexExampleModel is what the Codex backend calls the model.
	codexExampleModel = "gpt-5.6-sol"
	// codexExampleContextLength is the window the example block writes.
	codexExampleContextLength = 400000
)

// codexExampleBlock is one more [[models]] block, written entirely as comments,
// for OpenAI's Codex backend on the ChatGPT subscription. "nerdgenie init" cannot
// set it up itself yet, so the example is how a person learns the provider
// exists and turns it on in one edit. Every line is a comment, so the file
// loads exactly as it would without the block.
func codexExampleBlock() string {
	written := &strings.Builder{}
	written.WriteString("\n# One more model you can add: OpenAI's Codex backend on the ChatGPT\n")
	written.WriteString("# subscription, reached with the login the codex program keeps on this\n")
	written.WriteString("# machine, so that Nerd Genie's own loop drives the model rather than handing the\n")
	written.WriteString("# turn to the program. No key and no address are needed. To turn it on,\n")
	written.WriteString("# uncomment the lines below and name \"" + codexExampleAlias + "\" in default_model or fallback_chain.\n")
	written.WriteString("# [[models]]\n")
	fmt.Fprintf(written, "# name = %s\n", quoted(codexExampleAlias))
	fmt.Fprintf(written, "# provider = %s\n", quoted(string(contract.ProviderCodex)))
	fmt.Fprintf(written, "# model_name = %s\n", quoted(codexExampleModel))
	fmt.Fprintf(written, "# context_length = %d\n", codexExampleContextLength)
	fmt.Fprintf(written, "# think = %s\n", quoted(string(contract.ThinkMedium)))
	return written.String()
}

// capsBlock is the [caps] table of the file, with every budget written as a
// comment: the three are off unless the user sets one, because Nerd Genie puts no
// cap on its own work, and a person who wants one takes the "#" off the line.
// The table header is written out so that an uncommented line lands in the
// right table rather than at the top level, where the loader would refuse it.
func capsBlock() string {
	return "\n# How much one task may spend before Nerd Genie stops it and reports what is\n" +
		"# left. All three are off unless you set them: Nerd Genie puts no cap on its own\n" +
		"# work. To turn one on, take the \"#\" off its line and give it a number above\n" +
		"# zero, such as the ones shown. A command that hangs is still killed after\n" +
		"# time_per_tool, seven minutes, which is a safety limit and not a budget.\n" +
		"[caps]\n" +
		"# rounds_per_task = 100\n" +
		"# time_per_task = \"1h\"\n" +
		"# time_per_turn = \"15m\"\n"
}

// fallbackNames are the models to try when the chosen one cannot be reached:
// every other model found on this machine, in the order the menu offered them.
func fallbackNames(chosen modelChoice, found []modelChoice) []string {
	names := []string{}
	for _, choice := range found {
		if choice.detected && choice.name != chosen.name {
			names = append(names, choice.name)
		}
	}
	return names
}

// aliasesToWrite are the model blocks the file gets: the one chosen and every
// one found on this machine, so that the fallback chain names blocks that exist.
func aliasesToWrite(chosen modelChoice, found []modelChoice) []modelChoice {
	written := []modelChoice{}
	for _, choice := range found {
		if choice.detected || choice.name == chosen.name {
			written = append(written, choice)
		}
	}
	return written
}

// aliasBlock is one [[models]] block, with a comment above every line. A server
// on this machine that was not answering when "nerdgenie init" ran still gets its
// block, with a comment saying so, because that is how a person who has not
// started the daemon yet ends up with a configuration they can use.
func aliasBlock(choice modelChoice) string {
	alias := choice.alias
	written := &strings.Builder{}
	written.WriteString("\n# One model Nerd Genie can talk to. Add a block like this for another.\n")
	if !choice.detected && !choice.needsKey {
		written.WriteString("# This server was not answering when \"nerdgenie init\" ran. Start it, then run\n")
		written.WriteString("# \"nerdgenie doctor\" to check that Nerd Genie can reach it.\n")
	}
	written.WriteString("[[models]]\n")
	written.WriteString("# The short name you call this model by.\n")
	fmt.Fprintf(written, "name = %s\n", quoted(alias.Name))
	written.WriteString("# How it is reached: \"anthropic\", \"openai\" for any OpenAI-compatible\n")
	written.WriteString("# server, \"cli\" for the vendor's own program on your subscription, or\n")
	written.WriteString("# \"codex\" for OpenAI's Codex backend through the codex program's login.\n")
	fmt.Fprintf(written, "provider = %s\n", quoted(string(alias.Provider)))
	if alias.BaseAddress != "" {
		written.WriteString("# The address of the server that answers it.\n")
		fmt.Fprintf(written, "base_address = %s\n", quoted(alias.BaseAddress))
	}
	if alias.Program != "" {
		written.WriteString("# The program to run, already signed in to your subscription.\n")
		fmt.Fprintf(written, "program = %s\n", quoted(alias.Program))
	}
	written.WriteString("# What the server or the program calls the model.\n")
	fmt.Fprintf(written, "model_name = %s\n", quoted(alias.ModelName))
	written.WriteString("# How many tokens the model can hold, which is what the working context\n")
	written.WriteString("# is sized from. Set it to the window your model really has.\n")
	fmt.Fprintf(written, "context_length = %d\n", alias.ContextLength)
	written.WriteString("# How hard this model thinks before it answers: " + contract.ThinkLevelsSentence() + ".\n")
	written.WriteString("# Leave it empty to let the model think the way it does on its own. The\n")
	written.WriteString("# \"/think\" command changes it for one session without editing this file.\n")
	fmt.Fprintf(written, "think = %s\n", quoted(string(alias.Think)))
	if alias.KeyReference != "" {
		written.WriteString("# Where the API key is. The key itself lives in the vault and is never\n")
		written.WriteString("# written here; \"nerdgenie init\" put it there.\n")
		fmt.Fprintf(written, "key_reference = %s\n", quoted(alias.KeyReference))
	}
	return written.String()
}

// quoted writes one value the way TOML wants it, with anything inside it
// escaped, so that a folder name with a quotation mark in it cannot break the
// file.
func quoted(value string) string { return strconv.Quote(value) }

// quotedList writes a list of values the way TOML wants it.
func quotedList(values []string) string {
	written := make([]string, 0, len(values))
	for _, value := range values {
		written = append(written, quoted(value))
	}
	return "[" + strings.Join(written, ", ") + "]"
}

package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// personaFile is one of the three files that say who the agent is and who the
// user is: its path inside the home folder and the one line it explains itself
// with, so that a person opening it knows what to write there.
type personaFile struct {
	path string
	text string
}

// personaFiles returns the three files "coeus init" writes, each with a heading
// and one line saying what belongs in it.
func personaFiles(home contract.Home) []personaFile {
	return []personaFile{{
		path: home.SoulFile(),
		text: "# Who Coeus is\n\n" +
			"Write here who Coeus is and how it should behave, in your own words. It is read at the top of every call, so keep it short.\n",
	}, {
		path: home.UserFactsFile(),
		text: "# Who you are\n\n" +
			"Write here the facts about you that Coeus should always know, such as your name, where you are, and how you like to be answered.\n",
	}, {
		path: home.WorldFactsFile(),
		text: "# What Coeus knows about the world\n\n" +
			"Write here the facts that are not about you, such as the names of your machines and the sites Coeus works on. Coeus adds to this file as it learns.\n",
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

// configurationText is the config.toml a fresh install starts from: the model
// chosen, every other model that was found on this machine as a fallback, the
// folders Coeus may work in, and a comment above every line saying what it does.
func configurationText(chosen modelChoice, found []modelChoice, roots []string) string {
	written := &strings.Builder{}
	written.WriteString("# The Coeus configuration, written by \"coeus init\".\n")
	written.WriteString("# Every setting has a default, so a line you delete goes back to the default\n")
	written.WriteString("# rather than switching anything off. Run \"coeus doctor\" after editing it.\n\n")

	written.WriteString("# The model Coeus talks to when nothing else says otherwise.\n")
	fmt.Fprintf(written, "default_model = %s\n\n", quoted(chosen.name))

	written.WriteString("# The models to try, in order, when the one above cannot be reached.\n")
	fmt.Fprintf(written, "fallback_chain = %s\n\n", quotedList(fallbackNames(chosen, found)))

	written.WriteString("# The only folders a sandboxed command and the file tools may reach.\n")
	written.WriteString("# Everything else on this machine is outside the fence, and your home\n")
	written.WriteString("# directory as a whole is refused, because it holds your browser profile,\n")
	written.WriteString("# your cloud credentials, and your keys.\n")
	fmt.Fprintf(written, "sandbox_roots = %s\n", quotedList(roots))

	for _, alias := range aliasesToWrite(chosen, found) {
		written.WriteString(aliasBlock(alias))
	}
	return written.String()
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
func aliasesToWrite(chosen modelChoice, found []modelChoice) []contract.ModelAlias {
	written := []contract.ModelAlias{}
	for _, choice := range found {
		if choice.detected || choice.name == chosen.name {
			written = append(written, choice.alias)
		}
	}
	return written
}

// aliasBlock is one [[models]] block, with a comment above every line.
func aliasBlock(alias contract.ModelAlias) string {
	written := &strings.Builder{}
	written.WriteString("\n# One model Coeus can talk to. Add a block like this for another.\n[[models]]\n")
	written.WriteString("# The short name you call this model by.\n")
	fmt.Fprintf(written, "name = %s\n", quoted(alias.Name))
	written.WriteString("# How it is reached: \"anthropic\", \"openai\" for any OpenAI-compatible\n")
	written.WriteString("# server, or \"cli\" for the vendor's own program on your subscription.\n")
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
	if alias.KeyReference != "" {
		written.WriteString("# Where the API key is. The key itself lives in the vault and is never\n")
		written.WriteString("# written here; \"coeus init\" put it there.\n")
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

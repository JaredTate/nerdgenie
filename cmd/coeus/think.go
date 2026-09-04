package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// thinkName is the slash command that shows and sets how hard the model thinks.
const thinkName = "think"

// thinkCommand is the "/think" command: on its own it says how hard the model in
// use is thinking and names the levels, and with a level after it, it asks that
// model to think at that level for the rest of the session.
//
// Setting a level writes it in two places, and they are the same fact seen from
// two sides. It goes onto the model alias the program is running on, which is
// where "/status" reads it and where a chain built again after "/model" reads
// it, and it goes to the loop, which puts it on every call from here on, so that
// the providers already built are told without being built again.
func (running *agent) thinkCommand() contract.Command {
	return contract.Command{
		Name: thinkName,
		Help: "Shows how hard the model in use thinks before it answers, and sets it: /think medium.",
		Run: func(_ context.Context, arguments string, _ contract.CommandContext) (string, error) {
			asked := strings.ToLower(strings.TrimSpace(arguments))
			if asked == "" {
				return running.theThinkingLine(), nil
			}
			return running.useTheThinkLevel(contract.Think(asked))
		},
	}
}

// theThinkingLine is what "/think" on its own answers: the model in use, how
// hard it is thinking now, and every level a person may ask for.
func (running *agent) theThinkingLine() string {
	name := running.currentModel()
	return fmt.Sprintf("%s thinks at %s. The levels are %s; set one with \"/think medium\".",
		name, thinkLevelInWords(running.thinkLevelOf(name)), contract.ThinkLevelsSentence())
}

// thinkLevelInWords says a level the way a sentence says it, with the empty
// level written out rather than left as a gap in the middle of the line.
func thinkLevelInWords(level contract.Think) string {
	if level == contract.ThinkDefault {
		return "the level its provider chooses for itself"
	}
	return string(level)
}

// thinkLevelOf is how hard one model alias is set to think right now, which is
// what config.toml said unless "/think" has changed it this session.
func (running *agent) thinkLevelOf(name string) contract.Think {
	alias, found := aliasNamed(running.settings, name)
	if !found {
		return contract.ThinkDefault
	}
	return alias.Think
}

// useTheThinkLevel asks the model in use to think at one level for the rest of
// the session, refusing a level nobody offers and a level this model cannot be
// set to, such as switching the claude program's thinking off, which that
// program has no way to do.
func (running *agent) useTheThinkLevel(level contract.Think) (string, error) {
	name := running.currentModel()
	alias, found := aliasNamed(running.settings, name)
	if !found {
		return "", fmt.Errorf("config.toml names no model called %q, so run /model on its own to see the ones it does name", name)
	}
	if err := provider.CheckThink(alias, level); err != nil {
		return "", err
	}
	running.rememberTheThinkLevel(name, level)
	return fmt.Sprintf("%s thinks at %s from now on, and goes back to what config.toml says when Coeus starts again.",
		name, thinkLevelInWords(level)), nil
}

// rememberTheThinkLevel writes one level onto the model alias the program is
// running on and hands it to the loop, so that the next call carries it.
func (running *agent) rememberTheThinkLevel(name string, level contract.Think) {
	running.busyGuard.Lock()
	for at := range running.settings.Models {
		if running.settings.Models[at].Name == name {
			running.settings.Models[at].Think = level
		}
	}
	running.busyGuard.Unlock()
	running.loop.UseThink(name, level)
}

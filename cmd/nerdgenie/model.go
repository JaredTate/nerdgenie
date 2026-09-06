package main

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// openModel builds the model the configuration's default alias names, wrapped in
// retries, with the fallback chain behind it.
func (running *agent) openModel(ctx context.Context) (contract.Model, error) {
	return openTheChain(ctx, running, running.settings)
}

// openTheChain builds the model one configuration names, with its fallback chain
// behind it. It takes the settings rather than reading the agent's own, so that
// "/model" can build a chain for another alias without changing anything else.
func openTheChain(ctx context.Context, running *agent, settings contract.Config) (contract.Model, error) {
	options := provider.Options{Clock: clock.System(), Home: running.home, Log: running.note, OnReset: running.withdrawStreamedReply, Unseen: running.wroteUnseen}
	models := []contract.Model{}

	for _, name := range append([]string{settings.DefaultModel}, settings.FallbackChain...) {
		alias, found := aliasNamed(settings, name)
		if !found {
			return nil, fmt.Errorf("config.toml names %q as a model to use, and no models block defines it, so add one or change the name", name)
		}
		key, err := running.keyFor(ctx, alias)
		if err != nil {
			return nil, err
		}
		withKey := options
		withKey.APIKey = key
		one, err := provider.New(alias, withKey)
		if err != nil {
			return nil, err
		}
		models = append(models, provider.WithRetries(one, withKey))
	}
	return provider.NewChain(models, options)
}

// aliasNamed finds one model alias in the configuration by its name.
func aliasNamed(settings contract.Config, name string) (contract.ModelAlias, bool) {
	for _, alias := range settings.Models {
		if alias.Name == name {
			return alias, true
		}
	}
	return contract.ModelAlias{}, false
}

// keyFor reads a model's API key out of the vault, which is where the key lives
// and where the model never sees it. An alias with no key reference needs none,
// which is every local server and both subscription programs.
func (running *agent) keyFor(ctx context.Context, alias contract.ModelAlias) (string, error) {
	if alias.KeyReference == "" {
		return "", nil
	}
	found, err := running.secrets.Resolve(ctx, alias.KeyReference)
	if err != nil {
		return "", fmt.Errorf("the model alias %q needs the key at %s, and the vault could not give it: %w", alias.Name, alias.KeyReference, err)
	}
	return found.Password, nil
}

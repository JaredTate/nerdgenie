package command

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/vault"
)

// signalProgram is the program Coeus talks to Signal through, whose presence
// decides whether the Signal question is worth asking.
const signalProgram = "signal-cli"

// makeTheLayout makes every folder of the home layout with the mode the
// contract gives it, and writes the three persona files.
func makeTheLayout(home contract.Home) error {
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			return fmt.Errorf("the folder %s could not be made, so check that you can write in your home directory: %w", folder, err)
		}
		if err := os.Chmod(folder, contract.HomeFolderMode); err != nil {
			return fmt.Errorf("the folder %s could not be closed to other accounts, so check who owns it: %w", folder, err)
		}
	}
	return writePersonaFiles(home)
}

// askWorkFolders asks which folders Coeus may work in, checking every answer
// against the same rule the configuration checks it against, so that the whole
// home directory is refused here with the reason rather than at the next start.
func (setup Setup) askWorkFolders(ctx context.Context, ask *asker, chosen initFlags) ([]string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("your home directory could not be found, and the folders Coeus works in are measured from it, so set the HOME variable: %w", err)
	}
	fallback := contract.DefaultSandboxRoots(userHome)

	if len(chosen.workFolders) > 0 {
		return makeWorkFolders(expandFolders(chosen.workFolders, userHome), userHome)
	}
	if !ask.canAsk() {
		return makeWorkFolders(fallback, userHome)
	}

	question := "Which folders may Coeus work in? Nothing outside them can be read or written.\nSeparate several with commas."
	for tries := 0; tries < maxTriesPerQuestion; tries++ {
		answer, err := ask.line(ctx, question, strings.Join(fallback, ", "))
		if err != nil {
			return nil, err
		}
		roots, err := makeWorkFolders(expandFolders(splitFolders(answer), userHome), userHome)
		if err == nil {
			return roots, nil
		}
		fmt.Fprintf(ask.output, "%v\n", err)
	}
	return nil, fmt.Errorf("no folder Coeus may work in was given in %d tries, so nothing was set up", maxTriesPerQuestion)
}

// expandFolders turns every answer into a full path.
func expandFolders(written []string, userHome string) []string {
	folders := make([]string, 0, len(written))
	for _, one := range written {
		folders = append(folders, expandFolder(one, userHome))
	}
	return folders
}

// makeWorkFolders checks each folder against the sandbox-root rule and makes
// the ones that are not there yet, which is how the work folder ~/coeus comes
// into being on a fresh machine.
func makeWorkFolders(roots []string, userHome string) ([]string, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("no folder was named, so Coeus could reach nothing; name one such as %s", contract.DefaultSandboxRoots(userHome)[0])
	}
	for _, root := range roots {
		if err := contract.CheckSandboxRoot(root, userHome); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
			return nil, fmt.Errorf("the work folder %s could not be made, so check that you can write there: %w", root, err)
		}
	}
	return roots, nil
}

// askModel asks which model to use, from a numbered menu of what was found on
// this machine plus the two APIs that need a key.
func (setup Setup) askModel(ctx context.Context, ask *asker, chosen initFlags, found []modelChoice) (modelChoice, error) {
	if chosen.model != "" {
		picked, known := byName(found, chosen.model)
		if !known {
			return modelChoice{}, fmt.Errorf("--model is %q, so use one of %s", chosen.model, inPlainList(choiceNames(found)))
		}
		return picked, nil
	}

	menu := offered(found)
	if !ask.canAsk() {
		picked, any := firstDetected(found)
		if !any {
			return modelChoice{}, fmt.Errorf("no model was found on this machine and there was nobody to ask, so run coeus init again with --model %s and --api-key-from-env", inPlainList(choiceNames(found)))
		}
		return picked, nil
	}

	lines := make([]string, 0, len(menu))
	for _, choice := range menu {
		lines = append(lines, choice.description)
	}
	at, err := ask.choice(ctx, "Which model should Coeus use?", lines, 0)
	if err != nil {
		return modelChoice{}, err
	}
	return menu[at], nil
}

// storeKey puts the API key of a model that needs one into the vault, so that
// config.toml holds only a reference and the model never sees the key itself.
func (setup Setup) storeKey(ctx context.Context, chosen initFlags, picked modelChoice) error {
	if !picked.needsKey {
		return nil
	}
	key, err := setup.readKey(ctx, chosen, picked)
	if err != nil {
		return err
	}

	opened, err := vault.Open(setup.Home, clock.System())
	if err != nil {
		return fmt.Errorf("the vault could not be opened, so the key was not kept: %w", err)
	}
	defer func() { _ = opened.Close() }()
	if err := opened.Add(vault.Entry{Name: picked.name, Site: picked.description, Password: key}); err != nil {
		return fmt.Errorf("the key could not be put in the vault: %w", err)
	}
	return nil
}

// readKey gets the key itself, from the environment variable the flag named or
// from the masked prompt, and never from the command line.
func (setup Setup) readKey(_ context.Context, chosen initFlags, picked modelChoice) (string, error) {
	if chosen.apiKeyFromEnvironment != "" {
		key := strings.TrimSpace(os.Getenv(chosen.apiKeyFromEnvironment))
		if key == "" {
			return "", fmt.Errorf("the environment variable %s is empty, so set it to the %s key and run coeus init again", chosen.apiKeyFromEnvironment, picked.name)
		}
		return key, nil
	}
	if setup.AskSecret == nil {
		return "", fmt.Errorf("%s needs an API key and there is no terminal to type it in, so run coeus init again with --api-key-from-env naming the variable that holds it", picked.name)
	}

	key, err := setup.AskSecret(fmt.Sprintf("the %s API key (it is not shown as you type): ", picked.name))
	if err != nil {
		return "", fmt.Errorf("the %s key was not entered, so nothing was kept: %w", picked.name, err)
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("no %s key was entered, so run coeus init again and type it, or use --api-key-from-env", picked.name)
	}
	return strings.TrimSpace(key), nil
}

// askSignal asks whether the user wants to talk to Coeus over Signal too, which
// is only worth asking when signal-cli is installed.
func (setup Setup) askSignal(ctx context.Context, ask *asker, chosen initFlags) (bool, error) {
	if chosen.signal != "" {
		return chosen.signal == "on", nil
	}
	if !onThePath(signalProgram) {
		return false, nil
	}
	if !ask.canAsk() {
		return true, nil
	}
	return ask.yesOrNo(ctx, "signal-cli is installed. Do you want to talk to Coeus over Signal too?", true)
}

// writeConfiguration writes config.toml from the template this package owns.
func writeConfiguration(home contract.Home, picked modelChoice, found []modelChoice, roots []string) error {
	text := configurationText(picked, found, roots)
	if err := os.WriteFile(home.ConfigFile(), []byte(text), contract.DataFileMode); err != nil {
		return fmt.Errorf("the configuration file %s could not be written, so check that the home folder is writable: %w", home.ConfigFile(), err)
	}
	return nil
}

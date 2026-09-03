package command

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// PurgeConfirmation is the word that has to be typed, exactly and with nothing
// else on the line, before "coeus uninstall --purge" removes the home folder.
// Everything Coeus has learned is in there, so a slip of the finger must not be
// enough.
const PurgeConfirmation = "delete"

// systemctlWait is how long one call to the service manager may take. It is a
// local program answering about a local unit, so a call that has not come back
// in this time is not going to.
const systemctlWait = 30 * time.Second

// Uninstall stops the service, takes the unit away, and leaves the home folder
// where it is unless --purge is given and the confirmation word is typed.
func (service Service) Uninstall(ctx context.Context, arguments []string) error {
	if service.Output == nil {
		return errors.New("coeus uninstall has nowhere to print what it did, so give the service an output writer")
	}
	purge := false
	set := flag.NewFlagSet("coeus uninstall", flag.ContinueOnError)
	set.SetOutput(service.Output)
	set.BoolVar(&purge, "purge", false, "remove the home folder and everything Coeus has learned, after asking")
	if err := set.Parse(arguments); err != nil {
		return fmt.Errorf("coeus uninstall could not read its flags, so nothing was changed: %w", err)
	}
	if left := set.Args(); len(left) > 0 {
		return fmt.Errorf("coeus uninstall takes no plain words, and was given %q, so run it with --purge or with nothing at all", strings.Join(left, " "))
	}

	for _, unit := range Units(service.Home) {
		if !unit.Started {
			continue
		}
		for _, told := range [][]string{{"stop", unit.Name}, {"disable", unit.Name}} {
			if err := runSystemctl(ctx, told); err != nil {
				fmt.Fprintf(service.Output, "%v\n", err)
			}
		}
	}
	unitPath, err := removeUnits(service.Home)
	if err != nil {
		return err
	}
	if err := runSystemctl(ctx, []string{"daemon-reload"}); err != nil {
		fmt.Fprintf(service.Output, "%v\n", err)
	}
	fmt.Fprintf(service.Output, "stopped %s and removed %s.\n", ServiceName, unitPath)

	if !purge {
		fmt.Fprintf(service.Output, "%s was kept. Run \"coeus uninstall --purge\" to remove it too.\n", service.Home.Root)
		return nil
	}
	return service.purgeHome()
}

// purgeHome asks for the confirmation word and removes the home folder only
// when exactly that word was typed.
func (service Service) purgeHome() error {
	fmt.Fprintf(service.Output, "\n%s holds every task, memory, skill, and secret Coeus has.\n", service.Home.Root)
	fmt.Fprintf(service.Output, "Type %s to remove it, or press Enter to keep it.\n", PurgeConfirmation)

	typed, err := readConfirmation(service.Input)
	if err != nil {
		return err
	}
	if typed != PurgeConfirmation {
		fmt.Fprintf(service.Output, "%s was kept, because %q is not %s.\n", service.Home.Root, typed, PurgeConfirmation)
		return nil
	}
	if err := os.RemoveAll(service.Home.Root); err != nil {
		return fmt.Errorf("the home folder %s could not be removed, so check who owns it: %w", service.Home.Root, err)
	}
	fmt.Fprintf(service.Output, "removed %s.\n", service.Home.Root)
	return nil
}

// readConfirmation reads the one line the confirmation is typed on. Only the
// line ending is taken off, so that a word with a space around it is not the
// word.
func readConfirmation(input io.Reader) (string, error) {
	if input == nil {
		return "", errors.New("coeus uninstall --purge has nobody to ask, so run it in a terminal where you can type the confirmation")
	}
	line, err := bufio.NewReader(io.LimitReader(input, maxConversationBytes)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("the confirmation could not be read, so nothing was removed: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// runSystemctl tells the user's own service manager to do one thing, and says
// what it printed when it refuses.
func runSystemctl(ctx context.Context, arguments []string) error {
	waiting, stop := context.WithTimeout(ctx, systemctlWait)
	defer stop()

	told := append([]string{"--user"}, arguments...)
	running := exec.CommandContext(waiting, "systemctl", told...)
	printed, err := running.CombinedOutput()
	if err != nil {
		return fmt.Errorf("the service manager refused \"systemctl %s\", so check that systemd is running for your account: %s: %w",
			strings.Join(told, " "), strings.TrimSpace(string(printed)), err)
	}
	return nil
}

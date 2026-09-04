// What linking looks like was borrowed from OpenClaw's setup notes at
// ~/Code/openclaw/docs/channels/signal.md and from Hermes' Signal setup text in
// ~/Code/hermes-agent/hermes_cli/gateway.py, both of which tell the user to run
// "signal-cli link -n <name>" and scan what it prints. Neither draws the code
// itself; Nerd Genie does, so that the whole of linking happens in one place. The
// lines signal-cli prints were read from signal-cli 0.13.23.

package signal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/mdp/qrterminal/v3"
)

const (
	// LinkTimeout is how long the phone has to scan the code before the link
	// gives up and says so.
	LinkTimeout = 5 * time.Minute
	// DefaultDeviceName is the name this device shows up under in the phone's
	// list of linked devices.
	DefaultDeviceName = "nerdgenie"
	// linkingAddressPrefix is what signal-cli prints the address as.
	linkingAddressPrefix = "sgnl://"
	// linkedLinePrefix is the line signal-cli prints once the phone has scanned.
	linkedLinePrefix = "Associated with:"
	// maxLinkLines caps how many lines are read from signal-cli, so that a
	// program printing forever cannot hold the terminal open.
	maxLinkLines = 1000
	// linkFinishGrace is how long signal-cli is given to finish on its own once
	// the linking is done, before it is signalled.
	linkFinishGrace = 2 * time.Second
)

// LinkOptions is everything the link flow needs.
type LinkOptions struct {
	// Program is the signal-cli to run, either a path or a name on the PATH.
	Program string
	// DeviceName is what this device is called in the phone's list of linked
	// devices. Empty means "nerdgenie".
	DeviceName string
	// Out is where the code and the words around it are drawn.
	Out io.Writer
	// Clock is where the wait for the phone is measured.
	Clock contract.Clock
	// SaveAccount writes the linked account into the configuration. The
	// orchestrator supplies it in serve.go, and the subcommand supplies one that
	// writes config.toml.
	SaveAccount func(account string) error
}

// Link runs signal-cli's linking, draws the address it prints as a code for the
// phone to scan, waits for the phone with a deadline, and writes the account it
// was linked as into the configuration. It returns the account.
func Link(ctx context.Context, options LinkOptions) (string, error) {
	if err := checkLinkOptions(options); err != nil {
		return "", err
	}
	name := options.DeviceName
	if name == "" {
		name = DefaultDeviceName
	}

	running := exec.CommandContext(ctx, options.Program, "link", "-n", name)
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	printed, err := running.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("cannot read what signal-cli prints while linking: %w", err)
	}
	running.Stderr = running.Stdout
	if err := running.Start(); err != nil {
		return "", fmt.Errorf("cannot run signal-cli at %s, so install it and try again: %w", options.Program, err)
	}
	account, err := readLinking(ctx, options, printed)
	if err != nil {
		stopLinking(running)
		return "", err
	}
	finishLinking(running)

	if err := options.SaveAccount(account); err != nil {
		return "", fmt.Errorf("signal-cli linked this device as %s, but the account could not be written into the configuration: %w", account, err)
	}
	fmt.Fprintf(options.Out, "\nLinked as %s. Signal is ready.\n", account)
	return account, nil
}

// checkLinkOptions refuses options the link flow could not work with.
func checkLinkOptions(options LinkOptions) error {
	switch {
	case options.Program == "":
		return errors.New("linking has no program to run, so say where signal-cli is")
	case options.Out == nil:
		return errors.New("linking has nowhere to draw the code, so pass somewhere to write it")
	case options.Clock == nil:
		return errors.New("linking has no clock, and the wait for the phone is measured on one")
	case options.SaveAccount == nil:
		return errors.New("linking has nowhere to write the account, so pass the function that saves it")
	}
	return nil
}

// readLinking watches what signal-cli prints: first the address to draw, then
// the line saying which account the phone linked it to.
func readLinking(ctx context.Context, options LinkOptions, printed io.Reader) (string, error) {
	lines := make(chan string, streamLineBacklog)
	go readLinkLines(printed, lines)

	waited, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	expired := make(chan struct{})
	go func() {
		if err := options.Clock.Sleep(waited, LinkTimeout); err == nil {
			close(expired)
		}
	}()

	drawn := false
	for {
		select {
		case line, open := <-lines:
			if !open {
				return "", linkEndedError(drawn)
			}
			if account, found := readLinkLine(options, line, &drawn); found {
				return account, nil
			}
		case <-expired:
			return "", fmt.Errorf("no phone scanned the code within %v, so run \"nerdgenie signal link\" again and scan it from Signal on your phone under Linked Devices", LinkTimeout)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// readLinkLine looks at one line signal-cli printed, drawing the code when the
// address arrives and returning the account when the phone has scanned.
func readLinkLine(options LinkOptions, line string, drawn *bool) (string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case !*drawn && strings.HasPrefix(trimmed, linkingAddressPrefix):
		drawLinkingCode(options.Out, trimmed)
		*drawn = true
	case strings.HasPrefix(trimmed, linkedLinePrefix):
		account := strings.TrimSpace(strings.TrimPrefix(trimmed, linkedLinePrefix))
		if account != "" {
			return account, true
		}
	}
	return "", false
}

// linkEndedError says what to do when signal-cli stopped before the phone
// scanned anything.
func linkEndedError(drawn bool) error {
	if drawn {
		return errors.New("signal-cli stopped before the phone finished linking, so run \"nerdgenie signal link\" again and scan the code more quickly")
	}
	return errors.New("signal-cli printed no linking address, so check that signal-cli runs on its own and that this account is not already linked")
}

// drawLinkingCode writes the address as a square of blocks the phone's camera
// can read, and writes the address in words underneath for a terminal that
// cannot draw one.
func drawLinkingCode(out io.Writer, address string) {
	fmt.Fprintln(out, "Open Signal on your phone, go to Settings, then Linked Devices, and scan this:")
	fmt.Fprintln(out)
	qrterminal.GenerateHalfBlock(address, qrterminal.L, out)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "If your terminal cannot draw that, the address is:\n%s\n\n", address)
	fmt.Fprintln(out, "Waiting for your phone...")
}

// readLinkLines hands the lines signal-cli prints on, one at a time, and stops
// at the cap so that a program printing forever cannot hold the terminal.
func readLinkLines(printed io.Reader, lines chan<- string) {
	defer close(lines)
	scanner := bufio.NewScanner(printed)
	scanner.Buffer(make([]byte, 0, 4096), MaxEventBytes)
	for range maxLinkLines {
		if !scanner.Scan() {
			return
		}
		lines <- scanner.Text()
	}
}

// stopLinking ends signal-cli and everything it started at once, by the process
// group the child leads rather than by anything that looks like its name. It is
// what happens when the linking did not work.
func stopLinking(running *exec.Cmd) {
	if running.Process == nil {
		return
	}
	_ = syscall.Kill(-running.Process.Pid, syscall.SIGTERM)
	_ = running.Wait()
}

// finishLinking gives signal-cli a moment to finish on its own, because it says
// it has been linked just before it finishes writing the account down, and only
// signals it when it takes longer than that.
func finishLinking(running *exec.Cmd) {
	if running.Process == nil {
		return
	}
	finished := make(chan struct{})
	go func() {
		_ = running.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(linkFinishGrace):
		_ = syscall.Kill(-running.Process.Pid, syscall.SIGTERM)
		<-finished
	}
}

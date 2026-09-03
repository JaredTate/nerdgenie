// The unit file follows OpenClaw's at ~/Code/openclaw/src/daemon/systemd-unit.ts
// and ZeroClaw's service install at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/service/mod.rs: a user unit under
// ~/.config/systemd/user, started with "systemctl --user" after a daemon-reload,
// wanted by default.target, with the exit code for a bad configuration told not
// to restart. The exit codes themselves come from Hermes at
// ~/Code/hermes-agent/gateway/restart.py, where 75 asks the supervisor to start
// the program again and 78 says the configuration is wrong and a restart would
// fail the same way. What is added here is the watchdog line and the pointing of
// ExecStart at a link that never moves, so the updater can switch versions
// without rewriting the unit.

package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// ServiceName is the name of the systemd user unit Coeus runs under.
const ServiceName = "coeus.service"

// WatchdogSeconds is how long systemd waits to hear from Coeus before it
// decides that the program is stuck and starts it again.
const WatchdogSeconds = 60

// UnitText is the systemd user unit for one home folder. ExecStart points at
// the current link under the releases folder rather than at a version, so that
// the updater can switch versions by moving one link.
func UnitText(home contract.Home) string {
	return strings.Join([]string{
		"# Written by \"coeus install\". Run \"coeus install\" again to replace it, or",
		"# \"coeus uninstall\" to take it away.",
		"",
		"[Unit]",
		"Description=Coeus, an assistant that runs on your own computer",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=notify",
		"ExecStart=" + home.CurrentReleaseLink() + " serve",
		"Environment=COEUS_HOME=" + home.Root,
		fmt.Sprintf("WatchdogSec=%d", WatchdogSeconds),
		"Restart=on-failure",
		"RestartSec=5",
		fmt.Sprintf("SuccessExitStatus=%d", contract.ExitRestartMe),
		fmt.Sprintf("RestartForceExitStatus=%d", contract.ExitRestartMe),
		fmt.Sprintf("RestartPreventExitStatus=%d", contract.ExitBadConfiguration),
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
}

// UnitPath is where the unit file goes: the user's own systemd folder, which
// needs no administrator rights and starts with the user's session.
func UnitPath() (string, error) {
	configFolder, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("your configuration folder could not be found, so set the XDG_CONFIG_HOME or HOME variable and try again: %w", err)
	}
	return filepath.Join(configFolder, "systemd", "user", ServiceName), nil
}

// Service is what "coeus install" and "coeus uninstall" need from the outside.
type Service struct {
	// Home is the folder the service runs against.
	Home contract.Home
	// Program is the binary running right now, which the current link is made
	// to point at when there is no installed release yet.
	Program string
	// Input is where a typed confirmation is read from.
	Input io.Reader
	// Output is where what happened is printed.
	Output io.Writer
}

// Install writes the unit, makes the current link if there is none, and asks
// systemd to load, enable, and start the service.
func (service Service) Install(ctx context.Context) error {
	if service.Output == nil {
		return errors.New("coeus install has nowhere to print what it did, so give the service an output writer")
	}
	unitPath, err := UnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the folder %s for the unit could not be made: %w", filepath.Dir(unitPath), err)
	}
	if err := os.WriteFile(unitPath, []byte(UnitText(service.Home)), contract.DataFileMode); err != nil {
		return fmt.Errorf("the unit %s could not be written, so check that you can write in that folder: %w", unitPath, err)
	}
	if err := service.linkCurrentRelease(); err != nil {
		return err
	}

	for _, told := range [][]string{{"daemon-reload"}, {"enable", ServiceName}, {"start", ServiceName}} {
		if err := runSystemctl(ctx, told); err != nil {
			return err
		}
	}
	fmt.Fprintf(service.Output, "wrote %s and started %s.\n", unitPath, ServiceName)
	fmt.Fprintf(service.Output, "It runs %s, and systemd starts it again if it stops.\n", service.Home.CurrentReleaseLink())
	fmt.Fprintf(service.Output, "Run \"systemctl --user status %s\" to see how it is doing.\n", ServiceName)
	return nil
}

// linkCurrentRelease points the link the unit runs at the binary running right
// now, so that a machine with no installed release still has something to
// start. A link that is already there is left alone, because the updater owns it.
func (service Service) linkCurrentRelease() error {
	link := service.Home.CurrentReleaseLink()
	if _, err := os.Lstat(link); err == nil {
		return nil
	}
	if service.Program == "" {
		return errors.New("coeus install was not told which binary the service should run, so pass the path of the running program")
	}
	if err := os.MkdirAll(filepath.Dir(link), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the releases folder %s could not be made: %w", filepath.Dir(link), err)
	}
	if err := os.Symlink(service.Program, link); err != nil {
		return fmt.Errorf("the link %s could not be made to point at %s: %w", link, service.Program, err)
	}
	return nil
}

// removeUnit takes the unit file away, saying nothing when it was not there.
func removeUnit() (string, error) {
	unitPath, err := UnitPath()
	if err != nil {
		return "", err
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("the unit %s could not be removed, so check that you can write in that folder: %w", unitPath, err)
	}
	return unitPath, nil
}

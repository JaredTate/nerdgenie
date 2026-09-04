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

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ServiceName is the name of the systemd user unit Coeus runs under.
const ServiceName = "nerdgenie.service"

// The two units that run the nightly backup. The service does one backup and
// stops; the timer is what starts it, and is the one that is enabled.
const (
	// BackupServiceName is the unit that runs "nerdgenie backup" once.
	BackupServiceName = "nerdgenie-backup.service"
	// BackupTimerName is the timer that starts it every night.
	BackupTimerName = "nerdgenie-backup.timer"
	// backupTime is when the nightly backup runs, in the machine's own time
	// zone. Three in the morning is when the machine is least likely to be busy.
	backupTime = "*-*-* 03:00:00"
	// backupSpread is how long after that time the backup may actually start, so
	// that many machines do not all wake at once.
	backupSpread = 900
)

// WatchdogSeconds is how long systemd waits to hear from Coeus before it
// decides that the program is stuck and starts it again.
const WatchdogSeconds = 60

// writtenByInstall is the note at the top of every unit file, so that whoever
// finds one knows where it came from and how to take it away.
const writtenByInstall = "# Written by \"nerdgenie install\". Run \"nerdgenie install\" again to replace it, or\n" +
	"# \"nerdgenie uninstall\" to take it away."

// UnitText is the systemd user unit for one home folder. ExecStart points at
// the current link under the releases folder rather than at a version, so that
// the updater can switch versions by moving one link.
func UnitText(home contract.Home) string {
	return strings.Join([]string{
		writtenByInstall,
		"",
		"[Unit]",
		"Description=Coeus, an assistant that runs on your own computer",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=notify",
		"ExecStart=" + home.CurrentReleaseLink() + " serve",
		"Environment=NERDGENIE_HOME=" + home.Root,
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

// BackupServiceText is the unit that runs one backup and stops. It is started
// by the timer below and never enabled itself, which is how a systemd timer and
// the job it runs are written.
func BackupServiceText(home contract.Home) string {
	return strings.Join([]string{
		writtenByInstall,
		"",
		"[Unit]",
		"Description=Coeus nightly backup of the database, the vault, and the browser profile",
		"",
		"[Service]",
		"Type=oneshot",
		"ExecStart=" + home.CurrentReleaseLink() + " backup",
		"Environment=NERDGENIE_HOME=" + home.Root,
		"",
	}, "\n")
}

// BackupTimerText is the timer that runs the backup every night. It is
// persistent, so that a machine which was asleep at the time backs up when it
// comes back rather than skipping the night.
func BackupTimerText(home contract.Home) string {
	return strings.Join([]string{
		writtenByInstall,
		"",
		"[Unit]",
		"Description=Coeus nightly backup",
		"",
		"[Timer]",
		"OnCalendar=" + backupTime,
		fmt.Sprintf("RandomizedDelaySec=%d", backupSpread),
		"Persistent=true",
		"Unit=" + BackupServiceName,
		"",
		"[Install]",
		"WantedBy=timers.target",
		"",
	}, "\n")
}

// Unit is one systemd unit file that "nerdgenie install" writes.
type Unit struct {
	// Name is the file name, such as "nerdgenie.service".
	Name string
	// Text is what goes in the file.
	Text string
	// Started says the unit is enabled and started by "nerdgenie install" and
	// stopped and disabled by "nerdgenie uninstall". The backup service is not: the
	// timer starts it.
	Started bool
}

// Units are the three unit files "nerdgenie install" writes, in the order it writes
// them.
func Units(home contract.Home) []Unit {
	return []Unit{
		{Name: ServiceName, Text: UnitText(home), Started: true},
		{Name: BackupServiceName, Text: BackupServiceText(home)},
		{Name: BackupTimerName, Text: BackupTimerText(home), Started: true},
	}
}

// UnitPath is where the main unit file goes.
func UnitPath() (string, error) {
	return UnitPathFor(ServiceName)
}

// UnitPathFor is where one unit file goes: the user's own systemd folder, which
// needs no administrator rights and starts with the user's session.
func UnitPathFor(name string) (string, error) {
	configFolder, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("your configuration folder could not be found, so set the XDG_CONFIG_HOME or HOME variable and try again: %w", err)
	}
	return filepath.Join(configFolder, "systemd", "user", name), nil
}

// Service is what "nerdgenie install" and "nerdgenie uninstall" need from the outside.
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
		return errors.New("nerdgenie install has nowhere to print what it did, so give the service an output writer")
	}
	unitPath, err := UnitPath()
	if err != nil {
		return err
	}
	if err := writeUnits(service.Home); err != nil {
		return err
	}
	if err := service.linkCurrentRelease(); err != nil {
		return err
	}

	told := [][]string{{"daemon-reload"}}
	for _, unit := range Units(service.Home) {
		if unit.Started {
			told = append(told, []string{"enable", unit.Name}, []string{"start", unit.Name})
		}
	}
	for _, one := range told {
		if err := runSystemctl(ctx, one); err != nil {
			return err
		}
	}
	fmt.Fprintf(service.Output, "wrote %s and started %s.\n", unitPath, ServiceName)
	fmt.Fprintf(service.Output, "It runs %s, and systemd starts it again if it stops.\n", service.Home.CurrentReleaseLink())
	fmt.Fprintf(service.Output, "%s writes an encrypted backup every night.\n", BackupTimerName)
	fmt.Fprintf(service.Output, "Run \"systemctl --user status %s\" to see how it is doing.\n", ServiceName)
	return nil
}

// writeUnits puts every unit file in the user's own systemd folder.
func writeUnits(home contract.Home) error {
	for _, unit := range Units(home) {
		path, err := UnitPathFor(unit.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
			return fmt.Errorf("the folder %s for the unit could not be made: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(unit.Text), contract.DataFileMode); err != nil {
			return fmt.Errorf("the unit %s could not be written, so check that you can write in that folder: %w", path, err)
		}
	}
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
		return errors.New("nerdgenie install was not told which binary the service should run, so pass the path of the running program")
	}
	if err := os.MkdirAll(filepath.Dir(link), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the releases folder %s could not be made: %w", filepath.Dir(link), err)
	}
	if err := os.Symlink(service.Program, link); err != nil {
		return fmt.Errorf("the link %s could not be made to point at %s: %w", link, service.Program, err)
	}
	return nil
}

// removeUnits takes every unit file away, saying nothing about one that was not
// there, and returns where the main unit was.
func removeUnits(home contract.Home) (string, error) {
	unitPath, err := UnitPath()
	if err != nil {
		return "", err
	}
	for _, unit := range Units(home) {
		path, err := UnitPathFor(unit.Name)
		if err != nil {
			return "", err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("the unit %s could not be removed, so check that you can write in that folder: %w", path, err)
		}
	}
	return unitPath, nil
}

package contract

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// HomeFolderName is the folder the agent keeps everything in, under the user's
// own home directory.
const HomeFolderName = ".coeus"

// The modes every file and folder the agent creates is given. The home folder
// and everything holding a secret are readable by the agent's own user account
// and by nobody else, which is what makes the vault key safe.
const (
	// HomeFolderMode is the mode of every folder in the layout.
	HomeFolderMode fs.FileMode = 0o700
	// SecretFileMode is the mode of the vault, the vault key, and the backups.
	SecretFileMode fs.FileMode = 0o600
	// DataFileMode is the mode of an ordinary file, such as a memory note.
	DataFileMode fs.FileMode = 0o644
)

// Home is the agent's home folder and every path inside it. Nothing in Coeus
// builds one of these paths by hand, so that the layout lives in one place.
type Home struct {
	// Root is the folder itself, normally "~/.coeus".
	Root string
}

// NewHome returns the layout rooted at a folder, which is what a test uses to
// put a whole home under a temporary directory.
func NewHome(root string) Home {
	return Home{Root: root}
}

// DefaultHome returns the layout under the user's own home directory.
func DefaultHome() (Home, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Home{}, errors.New("cannot find your home directory, so set the HOME environment variable and try again")
	}
	return NewHome(filepath.Join(userHome, HomeFolderName)), nil
}

// ConfigFile is the one configuration file, read once at startup.
func (home Home) ConfigFile() string { return filepath.Join(home.Root, "config.toml") }

// DatabaseFile is the single SQLite file holding the event log, the records, the
// memory index, and the jobs.
func (home Home) DatabaseFile() string { return filepath.Join(home.Root, "coeus.db") }

// PersonaFolder holds the three files that say who the agent is and who the user
// is.
func (home Home) PersonaFolder() string { return filepath.Join(home.Root, "persona") }

// SoulFile describes who the agent is.
func (home Home) SoulFile() string { return filepath.Join(home.PersonaFolder(), "SOUL.md") }

// UserFactsFile holds the facts about the user.
func (home Home) UserFactsFile() string { return filepath.Join(home.PersonaFolder(), "USER.md") }

// WorldFactsFile holds the facts about the world.
func (home Home) WorldFactsFile() string { return filepath.Join(home.PersonaFolder(), "MEMORY.md") }

// MemoryFolder holds the memory notes too large for the two persona files.
func (home Home) MemoryFolder() string { return filepath.Join(home.Root, "memory") }

// SkillsFolder holds one folder per skill.
func (home Home) SkillsFolder() string { return filepath.Join(home.Root, "skills") }

// SkillFolder is one skill's folder.
func (home Home) SkillFolder(name string) string { return filepath.Join(home.SkillsFolder(), name) }

// ToolsFolder holds the user's own tools, one executable each.
func (home Home) ToolsFolder() string { return filepath.Join(home.Root, "tools") }

// VaultFile is the encrypted secret store.
func (home Home) VaultFile() string { return filepath.Join(home.Root, "vault.age") }

// VaultKeyFile is the key that opens the vault, readable by nobody else.
func (home Home) VaultKeyFile() string { return filepath.Join(home.Root, "vault.key") }

// BrowserFolder holds the Chrome profiles the agent uses, and is never the
// user's daily profile.
func (home Home) BrowserFolder() string { return filepath.Join(home.Root, "browser") }

// BrowserProfile is one Chrome profile folder.
func (home Home) BrowserProfile(name string) string {
	return filepath.Join(home.BrowserFolder(), name)
}

// InboxFolder holds the files and photos received over a channel.
func (home Home) InboxFolder() string { return filepath.Join(home.Root, "inbox") }

// ReleasesFolder holds the installed versions of the program.
func (home Home) ReleasesFolder() string { return filepath.Join(home.Root, "releases") }

// ReleaseFolder is one installed version.
func (home Home) ReleaseFolder(version string) string {
	return filepath.Join(home.ReleasesFolder(), version)
}

// CurrentReleaseLink is the link that says which installed version is live. The
// updater switches it, and switches it back when the new version does not come
// up.
func (home Home) CurrentReleaseLink() string {
	return filepath.Join(home.ReleasesFolder(), "current")
}

// RunFolder holds the socket and the lock, which do not survive a restart.
func (home Home) RunFolder() string { return filepath.Join(home.Root, "run") }

// SocketFile is the local socket every screen attaches to.
func (home Home) SocketFile() string { return filepath.Join(home.RunFolder(), "coeus.sock") }

// LockFile is what stops a second copy of the program from starting.
func (home Home) LockFile() string { return filepath.Join(home.RunFolder(), "coeus.lock") }

// BackupsFolder holds the nightly encrypted archives.
func (home Home) BackupsFolder() string { return filepath.Join(home.Root, "backups") }

// SignalFolder holds what the Signal channel keeps between runs: the pairing
// codes, hashed and salted, the approved senders, and the attachment cache.
func (home Home) SignalFolder() string { return filepath.Join(home.Root, "signal") }

// Folders lists every folder that must exist, in the order to create them.
func (home Home) Folders() []string {
	return []string{
		home.Root,
		home.PersonaFolder(),
		home.MemoryFolder(),
		home.SkillsFolder(),
		home.ToolsFolder(),
		home.BrowserFolder(),
		home.InboxFolder(),
		home.ReleasesFolder(),
		home.RunFolder(),
		home.BackupsFolder(),
		home.SignalFolder(),
	}
}

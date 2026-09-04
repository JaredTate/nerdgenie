package contract_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestHomePathsMatchTheLayoutInArchitecture(t *testing.T) {
	root := filepath.Join("/home", "someone", ".nerdgenie")
	home := contract.NewHome(root)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"the configuration file", home.ConfigFile(), filepath.Join(root, "config.toml")},
		{"the one database file", home.DatabaseFile(), filepath.Join(root, "nerdgenie.db")},
		{"the persona folder", home.PersonaFolder(), filepath.Join(root, "persona")},
		{"the agent's own description", home.SoulFile(), filepath.Join(root, "persona", "SOUL.md")},
		{"the facts about the user", home.UserFactsFile(), filepath.Join(root, "persona", "USER.md")},
		{"the facts about the world", home.WorldFactsFile(), filepath.Join(root, "persona", "MEMORY.md")},
		{"the memory folder", home.MemoryFolder(), filepath.Join(root, "memory")},
		{"the skills folder", home.SkillsFolder(), filepath.Join(root, "skills")},
		{"one skill's folder", home.SkillFolder("post-to-x"), filepath.Join(root, "skills", "post-to-x")},
		{"the user's own tools", home.ToolsFolder(), filepath.Join(root, "tools")},
		{"the vault", home.VaultFile(), filepath.Join(root, "vault.age")},
		{"the vault key", home.VaultKeyFile(), filepath.Join(root, "vault.key")},
		{"the browser folder", home.BrowserFolder(), filepath.Join(root, "browser")},
		{"one browser profile", home.BrowserProfile("default"), filepath.Join(root, "browser", "default")},
		{"the inbox", home.InboxFolder(), filepath.Join(root, "inbox")},
		{"the releases folder", home.ReleasesFolder(), filepath.Join(root, "releases")},
		{"one release", home.ReleaseFolder("1.2.0"), filepath.Join(root, "releases", "1.2.0")},
		{"the link to the live release", home.CurrentReleaseLink(), filepath.Join(root, "releases", "current")},
		{"the run folder", home.RunFolder(), filepath.Join(root, "run")},
		{"the local socket", home.SocketFile(), filepath.Join(root, "run", "agent.sock")},
		{"the lock", home.LockFile(), filepath.Join(root, "run", "nerdgenie.lock")},
		{"the backups folder", home.BackupsFolder(), filepath.Join(root, "backups")},
		{"the signal folder", home.SignalFolder(), filepath.Join(root, "signal")},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Errorf("%s is at %q, want %q", test.name, test.got, test.want)
		}
	}
}

func TestHomeFoldersListsEveryFolderThatMustExist(t *testing.T) {
	root := filepath.Join("/home", "someone", ".nerdgenie")
	home := contract.NewHome(root)

	folders := home.Folders()
	if len(folders) == 0 {
		t.Fatal("the list of folders to create is empty, and it should hold every folder in the layout")
	}
	for _, folder := range folders {
		if !strings.HasPrefix(folder, root) {
			t.Errorf("the folder %q is outside the home folder %q", folder, root)
		}
	}
	if !slices.Contains(folders, home.SignalFolder()) {
		t.Errorf("the signal folder %q is not in the list of folders to create", home.SignalFolder())
	}
}

func TestTheFileModesKeepSecretsReadableOnlyByTheAgentsUser(t *testing.T) {
	tests := []struct {
		name string
		mode fs.FileMode
		want fs.FileMode
	}{
		{"a folder in the home layout", contract.HomeFolderMode, 0o700},
		{"a file holding a secret", contract.SecretFileMode, 0o600},
		{"an ordinary data file", contract.DataFileMode, 0o644},
	}
	for _, test := range tests {
		if test.mode != test.want {
			t.Errorf("the mode for %s is %#o, want %#o", test.name, test.mode, test.want)
		}
	}
	if contract.SecretFileMode&0o077 != 0 {
		t.Errorf("the secret file mode %#o lets somebody other than the agent's user read it", contract.SecretFileMode)
	}
	if contract.HomeFolderMode&0o077 != 0 {
		t.Errorf("the home folder mode %#o lets somebody other than the agent's user in", contract.HomeFolderMode)
	}
}

func TestDefaultHomeSitsUnderTheUsersHomeDirectory(t *testing.T) {
	t.Setenv("HOME", filepath.Join("/home", "someone"))

	home, err := contract.DefaultHome()
	if err != nil {
		t.Fatalf("finding the default home folder failed: %v", err)
	}
	want := filepath.Join("/home", "someone", contract.HomeFolderName)
	if home.Root != want {
		t.Errorf("the default home folder is %q, want %q", home.Root, want)
	}
}

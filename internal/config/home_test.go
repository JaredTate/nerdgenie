package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheRootIsTheDotCoeusFolderUnderTheUsersHomeByDefault(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv(config.HomeVariable, "")

	root, err := config.Root()
	if err != nil {
		t.Fatalf("finding the home root with no %s set failed: %v", config.HomeVariable, err)
	}
	if root != home.Root {
		t.Errorf("the home root is %q, want %q, which is .coeus under the user's home directory", root, home.Root)
	}
}

func TestTheRootIsTheCoeusHomeVariableWhenItIsSet(t *testing.T) {
	testkit.NewTempHome(t)
	elsewhere := t.TempDir()
	t.Setenv(config.HomeVariable, elsewhere)

	root, err := config.Root()
	if err != nil {
		t.Fatalf("finding the home root with %s set to %q failed: %v", config.HomeVariable, elsewhere, err)
	}
	if root != elsewhere {
		t.Errorf("the home root is %q, want the folder %s names, %q", root, config.HomeVariable, elsewhere)
	}
}

func TestARelativeCoeusHomeIsRefusedWithAdviceToUseAFullPath(t *testing.T) {
	testkit.NewTempHome(t)
	t.Setenv(config.HomeVariable, "coeus-somewhere")

	_, err := config.Root()
	if err == nil {
		t.Fatal("a relative COEUS_HOME was accepted, want it refused because every path in the layout is built from the root")
	}
	if !strings.Contains(err.Error(), "coeus-somewhere") || !strings.Contains(err.Error(), config.HomeVariable) {
		t.Errorf("the error is %q, want it to name both the variable and the value that is wrong", err)
	}
}

func TestATrailingSeparatorIsTakenOffTheCoeusHome(t *testing.T) {
	testkit.NewTempHome(t)
	elsewhere := t.TempDir()
	t.Setenv(config.HomeVariable, elsewhere+string(filepath.Separator))

	root, err := config.Root()
	if err != nil {
		t.Fatalf("finding the home root with a trailing separator failed: %v", err)
	}
	if root != elsewhere {
		t.Errorf("the home root is %q, want %q with no trailing separator", root, elsewhere)
	}
}

func TestTheHomeFolderCarriesTheWholeLayoutFromTheRoot(t *testing.T) {
	testkit.NewTempHome(t)
	elsewhere := t.TempDir()
	t.Setenv(config.HomeVariable, elsewhere)

	home, err := config.HomeFolder()
	if err != nil {
		t.Fatalf("building the home layout failed: %v", err)
	}
	if home.Root != elsewhere {
		t.Errorf("the layout is rooted at %q, want %q", home.Root, elsewhere)
	}
	if want := filepath.Join(elsewhere, "config.toml"); home.ConfigFile() != want {
		t.Errorf("the configuration file is %q, want %q", home.ConfigFile(), want)
	}
	if want := filepath.Join(elsewhere, "vault.age"); home.VaultFile() != want {
		t.Errorf("the vault file is %q, want %q", home.VaultFile(), want)
	}
}

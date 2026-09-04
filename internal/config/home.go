package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// HomeVariable is the one environment variable Coeus reads. It moves the whole
// home folder somewhere else, which is what a test, a container, and a second
// copy on the same machine all need. Nothing else in the configuration can be
// set from the environment.
const HomeVariable = "NERDGENIE_HOME"

// Root returns the folder Coeus keeps everything in: the folder HomeVariable
// names when that variable is set to something, and .nerdgenie under the user's own
// home directory otherwise.
func Root() (string, error) {
	named := strings.TrimSpace(os.Getenv(HomeVariable))
	if named == "" {
		home, err := contract.DefaultHome()
		if err != nil {
			return "", err
		}
		return home.Root, nil
	}
	if !filepath.IsAbs(named) {
		return "", fmt.Errorf("the %s variable is set to %q, which is not a full path, so set it to a path starting from the root of the filesystem or unset it to use the default",
			HomeVariable, named)
	}
	return filepath.Clean(named), nil
}

// HomeFolder returns the whole layout from ARCHITECTURE.md, rooted where Root
// says. Every other package asks for its paths through this, so that the layout
// lives in one place.
func HomeFolder() (contract.Home, error) {
	root, err := Root()
	if err != nil {
		return contract.Home{}, err
	}
	return contract.NewHome(root), nil
}

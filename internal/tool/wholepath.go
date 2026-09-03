package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

// homeMarks are the ways a model writes the user's home folder at the front of a
// path. A model that has seen a shell writes any of them, and all three mean the
// same folder.
var homeMarks = []string{"~", "$HOME", "${HOME}"}

// MadeWhole returns a check that makes a path whole before the check it wraps
// judges it, because every model writes paths that are not whole: the first
// human trial saw haiku.txt refused by the write tool and ~/Desktop/... refused
// by the read tool. A path beginning with ~, $HOME or ${HOME} is read as the
// user's home folder, and a path that starts at neither the root of the
// filesystem nor the home is taken from the folder the agent works in. Only then
// does the wrapped check run, so such a path is refused only when it lands
// outside what that check allows, and the refusal then carries one more clause
// saying how the path was read, because a model that cannot see where its name
// landed cannot correct it. An empty working folder means a short path is
// refused here, since a step with nowhere to take it from cannot tell which file
// was meant, and with no home named a path beginning with ~ is an ordinary
// short path.
func MadeWhole(check PathCheck, workingFolder string, userHome string) PathCheck {
	workingFolder = strings.TrimSpace(workingFolder)
	userHome = strings.TrimSpace(userHome)
	return func(path string) (string, error) {
		whole, howItWasRead, err := madeWhole(path, workingFolder, userHome)
		if err != nil {
			return "", err
		}
		allowed, err := check(whole)
		if err != nil {
			return "", alsoSayHowItWasRead(err, howItWasRead)
		}
		return allowed, nil
	}
}

// madeWhole returns the whole path of what the model wrote, with one clause
// saying how it was read when it was not written whole, so that a refusal can
// say so. A path that is already whole, and one that names nothing at all,
// come back exactly as written for the wrapped check to judge, because nothing
// joined to a folder would be that folder, and a call that named no file must
// not be read as naming one.
func madeWhole(path string, workingFolder string, userHome string) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return path, "", nil
	}
	if underHome, isUnderHome := underTheHome(trimmed, userHome); isUnderHome {
		return underHome, fmt.Sprintf("the path %q begins with ~ or $HOME, which is read as the user's home folder, %s", trimmed, userHome), nil
	}
	if workingFolder == "" {
		return "", "", fmt.Errorf("the path %q does not start at the root of the filesystem, and this tool has no folder to take it from, so write the whole path", trimmed)
	}
	return filepath.Join(workingFolder, trimmed),
		fmt.Sprintf("the path %q does not start at the root of the filesystem, so it was taken from %s, the folder the agent works in", trimmed, workingFolder),
		nil
}

// underTheHome turns a path that begins with a mark for the home folder into a
// whole path under that home, and says whether it was one. With no home named
// such a path is left alone, because a home nobody named cannot be put in front
// of it.
func underTheHome(path string, userHome string) (string, bool) {
	if userHome == "" {
		return "", false
	}
	for _, mark := range homeMarks {
		if path == mark {
			return filepath.Clean(userHome), true
		}
		if rest, found := strings.CutPrefix(path, mark+"/"); found {
			return filepath.Join(userHome, rest), true
		}
	}
	return "", false
}

// alsoSayHowItWasRead adds to a refusal the one clause saying how a path that
// was not written whole was read, so that a model which wrote a bare name such
// as haiku.txt, or a path beginning with ~, can see where the name landed and
// why that was refused. A path that was already whole was read as it stood and
// gets no clause.
func alsoSayHowItWasRead(err error, howItWasRead string) error {
	if howItWasRead == "" {
		return err
	}
	return fmt.Errorf("%w; the path was read this way before it was judged: %s", err, howItWasRead)
}

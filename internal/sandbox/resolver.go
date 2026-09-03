package sandbox

import (
	"path/filepath"
	"strings"
)

// resolverFilePath is where every program on a Linux machine looks for the
// addresses of the name servers it should ask.
const resolverFilePath = "/etc/resolv.conf"

// resolverFileToBind returns the one extra file the fence has to bind so that a
// command inside it can look a name up, and an empty string when there is
// nothing extra to bind.
//
// Binding /etc read-only is not enough on a machine running systemd-resolved,
// because there /etc/resolv.conf is a link into /run, which the fence does not
// bind: inside the fence the link dangles, and only literal addresses work. So
// the link is followed here, and the file it leads to is bound at its own path
// as well.
//
// Nothing is bound when the settings are a plain file inside a folder the fence
// already binds, and nothing is bound when the link leads nowhere, because bwrap
// stops before it starts when a bind names a file that is not there.
func resolverFileToBind(path string, boundFolders []string) string {
	realFile, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	for _, folder := range boundFolders {
		if realFile == folder || strings.HasPrefix(realFile, folder+string(filepath.Separator)) {
			return ""
		}
	}
	return realFile
}

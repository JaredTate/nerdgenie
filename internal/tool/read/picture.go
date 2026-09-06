package read

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg" // so a JPEG's size can be read
	_ "image/png"  // so a PNG's size can be read
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxPictureBytes is the largest picture the tool hands back: a screenshot is
// a few hundred kilobytes, and a model reads nothing useful from more.
const MaxPictureBytes = 4 << 20

// isAPicture says whether a path names a picture by its ending: the two kinds
// the local daemon and the APIs all read.
func isAPicture(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}

// readPicture hands a picture file back as the picture itself, with one line
// naming the file, its size in pixels and its size in bytes. The thirteenth
// nightly run's polish task took a picture of its canvas and was refused it
// as not a text file.
func readPicture(path string) (contract.ToolOutput, error) {
	about, err := os.Stat(path)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot open %s, so check the path and try again: %w", path, err)
	}
	if about.Size() > MaxPictureBytes {
		return contract.ToolOutput{}, fmt.Errorf("%s is %d bytes, and a picture over %d bytes is not shown, so make a smaller one", path, about.Size(), MaxPictureBytes)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	shape, kind, err := image.DecodeConfig(strings.NewReader(string(bytes)))
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("%s does not read as a picture, so check the file: %w", path, err)
	}
	return contract.ToolOutput{
		Text:    fmt.Sprintf("the picture %s, a %s of %d by %d pixels, %d bytes\n", path, kind, shape.Width, shape.Height, len(bytes)),
		Picture: base64.StdEncoding.EncodeToString(bytes),
	}, nil
}

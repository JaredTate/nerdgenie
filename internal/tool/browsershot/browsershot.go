package browsershot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ThePictureIsSavedAt opens the line naming the file the picture was saved as.
const ThePictureIsSavedAt = "the picture is saved at "

// TheSamePictureLine is the result of a picture that is the same as the last
// one taken at its size: run 23's visual QA took eleven screenshots, several
// of the same board, and looked hard at each one. The same picture is said to
// be the same, with no picture to look at again and no new file.
const TheSamePictureLine = "the picture is the same as the last one at this size; nothing to look at again"

// MaxPicturesRemembered bounds how many sizes the tool remembers a last
// picture for.
const MaxPicturesRemembered = 16

// Settings is what the tool needs: the browser, and the folder screenshots
// are saved under, which may be empty.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// SavesTo is the folder screenshots are saved under, or empty.
	SavesTo string
}

// Tool is the browser_screenshot tool.
type Tool struct {
	settings Settings
	saved    int
	// last is the last picture taken at each size: its hash and the file it
	// was saved as, keyed by the picture's width and height.
	last map[string]lastPicture
	// sizes are the sizes remembered, oldest first, which is the order they
	// are forgotten in when the memory is full.
	sizes []string
}

// lastPicture is what is remembered of the last picture at one size.
type lastPicture struct {
	hash string
	path string
}

// New builds the tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings, last: map[string]lastPicture{}}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserScreenshot,
		Description: "Takes a picture of the page the browser is on, with its clickable elements numbered, " +
			"and shows it to you; use it to check what a page or a canvas really looks like.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this look is for, in one line.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork},
	}
}

// Run takes the picture, saves it, and hands it back with its numbered marks.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := struct {
		Intent string `json:"intent"`
	}{}
	if err := json.Unmarshal(written, &asked); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the arguments could not be read, so write intent as one line: %w", err)
	}
	if strings.TrimSpace(asked.Intent) == "" {
		return contract.ToolOutput{}, errors.New("this call names no intent, so say in one line what this look is for")
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("no browser is wired to this tool, so it cannot take a picture")
	}
	picture, err := tool.settings.Browser.Screenshot(ctx)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot take a picture of the page: %w", err)
	}
	if before, same := tool.theSameAsTheLast(picture.PNGBase64); same {
		said := TheSamePictureLine
		if before.path != "" {
			said += " (" + ThePictureIsSavedAt + before.path + ")"
		}
		return contract.ToolOutput{Text: said + "\n"}, nil
	}
	text := &strings.Builder{}
	path, err := tool.savePicture(picture.PNGBase64)
	if err == nil && path != "" {
		text.WriteString(ThePictureIsSavedAt + path + "\n")
	}
	tool.rememberThePicture(picture.PNGBase64, path)
	if len(picture.Marks) == 0 {
		text.WriteString("nothing on the page is numbered, because nothing on it can be clicked\n")
	} else {
		text.WriteString("numbered on the picture:\n")
		for _, mark := range picture.Marks {
			fmt.Fprintf(text, "%d %s %q (%s)\n", mark.Number, mark.Role, mark.Name, mark.Ref)
		}
	}
	return contract.ToolOutput{Text: text.String(), Picture: picture.PNGBase64}, nil
}

// theSameAsTheLast says whether the picture is the same as the last one taken
// at its size, and hands back what is remembered of that one.
func (tool *Tool) theSameAsTheLast(pngBase64 string) (lastPicture, bool) {
	if pngBase64 == "" {
		return lastPicture{}, false
	}
	before, held := tool.last[sizeOf(pngBase64)]
	return before, held && before.hash == hashOf(pngBase64)
}

// rememberThePicture keeps the picture's hash and file as the last one at
// its size, and forgets the oldest size when the memory is full.
func (tool *Tool) rememberThePicture(pngBase64 string, path string) {
	if pngBase64 == "" {
		return
	}
	size := sizeOf(pngBase64)
	if _, held := tool.last[size]; !held {
		if len(tool.sizes) >= MaxPicturesRemembered {
			delete(tool.last, tool.sizes[0])
			tool.sizes = tool.sizes[1:]
		}
		tool.sizes = append(tool.sizes, size)
	}
	tool.last[size] = lastPicture{hash: hashOf(pngBase64), path: path}
}

// hashOf is the picture's hash, which is what says two pictures are the same.
func hashOf(pngBase64 string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(pngBase64)))
}

// sizeOf reads the picture's width and height off the PNG header, which is
// what tells one viewport size from another; a picture with no readable
// header is one size on its own.
func sizeOf(pngBase64 string) string {
	head := make([]byte, 24)
	if _, err := base64.StdEncoding.Decode(head, []byte(pngBase64[:min(len(pngBase64), 32)])); err != nil || !bytes.HasPrefix(head, []byte("\x89PNG")) {
		return "unknown"
	}
	return fmt.Sprintf("%dx%d", binary.BigEndian.Uint32(head[16:20]), binary.BigEndian.Uint32(head[20:24]))
}

// savePicture writes the picture as the next numbered file under the folder
// the tool saves to, and hands back its path, or nothing when there is no
// folder or no picture.
func (tool *Tool) savePicture(pngBase64 string) (string, error) {
	if tool.settings.SavesTo == "" || pngBase64 == "" {
		return "", nil
	}
	bytes, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return "", fmt.Errorf("the picture is not base64, so it cannot be saved: %w", err)
	}
	if err := os.MkdirAll(tool.settings.SavesTo, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot make the folder for screenshots, so the picture is not saved: %w", err)
	}
	tool.saved++
	path := filepath.Join(tool.settings.SavesTo, fmt.Sprintf("screenshot-%d.png", tool.saved))
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		return "", fmt.Errorf("cannot write the screenshot, so the picture is not saved: %w", err)
	}
	return path, nil
}

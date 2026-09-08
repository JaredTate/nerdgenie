package browsershot_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browsershot"
)

// aWorkerWhosePictureChanges is the fake browser with a picture of the
// test's own choosing on every screenshot, in order.
type aWorkerWhosePictureChanges struct {
	contract.BrowserWorker
	pictures []string
}

func (worker *aWorkerWhosePictureChanges) Screenshot(ctx context.Context) (contract.Screenshot, error) {
	picture, err := worker.BrowserWorker.Screenshot(ctx)
	if err != nil {
		return picture, err
	}
	if len(worker.pictures) > 0 {
		picture.PNGBase64, worker.pictures = worker.pictures[0], worker.pictures[1:]
	}
	return picture, nil
}

// TestTheSamePictureTwiceIsSaidOnceWithoutThePicture: run 23's visual QA
// took eleven screenshots, several of the same board, and looked hard at each
// one. A picture that is the same as the last one at its size is said to be
// the same, with no picture to look at again and no new file; a picture that
// differs is handed back as before.
func TestTheSamePictureTwiceIsSaidOnceWithoutThePicture(t *testing.T) {
	inner := testkit.NewFakeBrowserWorker()
	if _, err := inner.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatal(err)
	}
	first, err := inner.Screenshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bytesOfFirst, err := base64.StdEncoding.DecodeString(first.PNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte{}, bytesOfFirst...)
	changed[len(changed)-1] ^= 0xff
	worker := &aWorkerWhosePictureChanges{BrowserWorker: inner, pictures: []string{first.PNGBase64, first.PNGBase64, base64.StdEncoding.EncodeToString(changed)}}
	folder := t.TempDir()
	tool := browsershot.New(browsershot.Settings{Browser: worker, SavesTo: folder})

	one, err := run(t, tool, map[string]any{"intent": "see the board"})
	if err != nil || one.Picture == "" {
		t.Fatalf("the first picture was not handed back: %v", err)
	}
	two, err := run(t, tool, map[string]any{"intent": "see the board again"})
	if err != nil {
		t.Fatal(err)
	}
	if two.Picture != "" || !strings.Contains(two.Text, browsershot.TheSamePictureLine) || !strings.Contains(two.Text, filepath.Join(folder, "screenshot-1.png")) {
		t.Errorf("the same picture again was handed back as new:\npicture %d bytes, text:\n%s", len(two.Picture), two.Text)
	}
	if _, err := os.Stat(filepath.Join(folder, "screenshot-2.png")); err == nil {
		t.Errorf("the same picture was saved a second time")
	}
	three, err := run(t, tool, map[string]any{"intent": "see the board after a move"})
	if err != nil {
		t.Fatal(err)
	}
	if three.Picture == "" || strings.Contains(three.Text, browsershot.TheSamePictureLine) || !strings.Contains(three.Text, filepath.Join(folder, "screenshot-2.png")) {
		t.Errorf("a different picture was not handed back:\n%s", three.Text)
	}
}

// aPictureOfSize is the header of a PNG of the given width and height, which
// is all the tool reads of a picture to tell one viewport size from another.
func aPictureOfSize(width int, height int) string {
	head := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")
	head = append(head, byte(width>>24), byte(width>>16), byte(width>>8), byte(width), byte(height>>24), byte(height>>16), byte(height>>8), byte(height))
	return base64.StdEncoding.EncodeToString(append(head, []byte("the rest of the picture")...))
}

// TestTheMemoryOfPicturesIsBoundedAndNeedsNoFolder: a picture is compared
// with the last one at its own size, the memory holds at most
// MaxPicturesRemembered sizes, a picture that is no PNG at all is one size
// of its own, and a tool with no folder to save into still says when the
// picture is the same, without naming a file.
func TestTheMemoryOfPicturesIsBoundedAndNeedsNoFolder(t *testing.T) {
	inner := testkit.NewFakeBrowserWorker()
	if _, err := inner.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatal(err)
	}
	pictures := []string{}
	for size := 1; size <= browsershot.MaxPicturesRemembered+1; size++ {
		pictures = append(pictures, aPictureOfSize(100*size, 100))
	}
	notAPicture := base64.StdEncoding.EncodeToString([]byte("not a picture at all"))
	pictures = append(pictures, aPictureOfSize(100, 100), notAPicture, notAPicture)
	worker := &aWorkerWhosePictureChanges{BrowserWorker: inner, pictures: pictures}
	tool := browsershot.New(browsershot.Settings{Browser: worker})

	for at := 0; at <= browsershot.MaxPicturesRemembered; at++ {
		output, err := run(t, tool, map[string]any{"intent": "see a size"})
		if err != nil || output.Picture == "" || strings.Contains(output.Text, browsershot.TheSamePictureLine) {
			t.Fatalf("picture %d of a new size was not handed back: %v\n%s", at+1, err, output.Text)
		}
	}
	first, err := run(t, tool, map[string]any{"intent": "see the first size again"})
	if err != nil || first.Picture == "" {
		t.Errorf("the first size was still remembered past the cap, or the picture was refused: %v", err)
	}
	odd, err := run(t, tool, map[string]any{"intent": "see something that is no picture"})
	if err != nil || odd.Picture == "" {
		t.Errorf("a picture that is no PNG was not handed back the first time: %v", err)
	}
	again, err := run(t, tool, map[string]any{"intent": "see it again"})
	if err != nil || again.Picture != "" || !strings.HasPrefix(again.Text, browsershot.TheSamePictureLine) || strings.Contains(again.Text, browsershot.ThePictureIsSavedAt) {
		t.Errorf("the same picture again, with no folder to save into, reads %q (%v)", again.Text, err)
	}
}

package signal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestAttachmentCacheKeepsTheBytesUnderTheSignalFolder(t *testing.T) {
	home := testkit.NewTempHome(t)
	cache, err := newAttachmentCache(home, AttachmentCacheLimit)
	if err != nil {
		t.Fatalf("cannot open the attachment cache: %v", err)
	}

	path, err := cache.save(Attachment{ID: "abc123", Filename: "photo.jpg"}, []byte("the photo"))
	if err != nil {
		t.Fatalf("saving an attachment failed: %v", err)
	}
	if !strings.HasPrefix(path, home.SignalFolder()) {
		t.Errorf("the attachment landed at %q, want it under the Signal folder %q", path, home.SignalFolder())
	}
	if filepath.Ext(path) != ".jpg" {
		t.Errorf("the attachment landed at %q, want it to keep the .jpg ending so a reader can tell what it is", path)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the saved attachment: %v", err)
	}
	if string(written) != "the photo" {
		t.Errorf("the saved attachment holds %q, want the bytes that were downloaded", written)
	}
}

func TestAttachmentCacheThrowsOutTheOldestWhenItIsFull(t *testing.T) {
	home := testkit.NewTempHome(t)
	cache, err := newAttachmentCache(home, 30)
	if err != nil {
		t.Fatalf("cannot open the attachment cache: %v", err)
	}

	oldest, err := cache.save(Attachment{ID: "one", Filename: "one.bin"}, []byte(strings.Repeat("a", 10)))
	if err != nil {
		t.Fatalf("saving the first attachment failed: %v", err)
	}
	ageTheFile(t, oldest, 2*time.Hour)
	middle, err := cache.save(Attachment{ID: "two", Filename: "two.bin"}, []byte(strings.Repeat("b", 10)))
	if err != nil {
		t.Fatalf("saving the second attachment failed: %v", err)
	}
	ageTheFile(t, middle, time.Hour)

	newest, err := cache.save(Attachment{ID: "three", Filename: "three.bin"}, []byte(strings.Repeat("c", 15)))
	if err != nil {
		t.Fatalf("saving the third attachment failed: %v", err)
	}

	if _, err := os.Stat(oldest); err == nil {
		t.Errorf("the oldest attachment is still there, and the oldest is what goes first when the cache is full")
	}
	if _, err := os.Stat(middle); err != nil {
		t.Errorf("the middle attachment was thrown out before the oldest one: %v", err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("the attachment just saved is not there: %v", err)
	}
}

func TestAttachmentCacheRefusesOneFileBiggerThanItself(t *testing.T) {
	home := testkit.NewTempHome(t)
	cache, err := newAttachmentCache(home, 10)
	if err != nil {
		t.Fatalf("cannot open the attachment cache: %v", err)
	}

	if _, err := cache.save(Attachment{ID: "huge", Filename: "huge.bin"}, []byte(strings.Repeat("a", 11))); err == nil {
		t.Errorf("the cache took a file bigger than the whole cache, and no room can ever be made for it")
	}
}

func TestAttachmentCacheCannotBeTalkedOutOfItsOwnFolder(t *testing.T) {
	home := testkit.NewTempHome(t)
	cache, err := newAttachmentCache(home, AttachmentCacheLimit)
	if err != nil {
		t.Fatalf("cannot open the attachment cache: %v", err)
	}

	nasty := []Attachment{
		{ID: "../../escaped", Filename: "escaped.txt"},
		{ID: "/etc/passwd", Filename: "passwd"},
		{ID: "..", Filename: "dots"},
		{ID: strings.Repeat("x", 500), Filename: "long.bin"},
	}
	for _, attachment := range nasty {
		path, err := cache.save(attachment, []byte("harmless"))
		if err != nil {
			continue
		}
		if filepath.Dir(path) != cache.folder {
			t.Errorf("an attachment named %q landed at %q, outside the cache folder %q", attachment.ID, path, cache.folder)
		}
	}
}

// ageTheFile moves a file's time backwards, so that a test can say which of two
// files in the cache is older without waiting.
func ageTheFile(t *testing.T, path string, by time.Duration) {
	t.Helper()
	when := time.Now().Add(-by)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("cannot change the time on %s: %v", path, err)
	}
}

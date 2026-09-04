package signal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// AttachmentCacheLimit is how many bytes of downloaded attachments are kept
	// under the home's Signal folder before the oldest are thrown out. Two
	// hundred megabytes holds months of ordinary photos and is small enough that
	// nobody notices it on a disk.
	AttachmentCacheLimit = 200 << 20
	// maxCachedNameLength caps how long the name of a cached file may be, so
	// that an identifier the daemon made up cannot run past what a filesystem
	// will take.
	maxCachedNameLength = 64
	// maxCachedFiles caps how many files the cache walks when making room, which
	// is what stops a folder somebody filled from stalling a download.
	maxCachedFiles = 10000
)

// attachmentCache is the folder downloaded attachments are kept in, with a cap
// on how much it may hold. When a new file will not fit, the files that have
// been there longest go first.
type attachmentCache struct {
	folder string
	limit  int64
}

// newAttachmentCache opens the cache folder under the home's Signal folder,
// making it when it is not there.
func newAttachmentCache(home contract.Home, limit int64) (*attachmentCache, error) {
	folder := filepath.Join(home.SignalFolder(), "attachments")
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return nil, fmt.Errorf("cannot make the attachment cache folder %s, so check that the home folder is writable: %w", folder, err)
	}
	return &attachmentCache{folder: folder, limit: limit}, nil
}

// save writes one downloaded attachment into the cache, making room first, and
// returns where it landed. A file bigger than the whole cache is refused,
// because no amount of throwing things out would make room for it.
func (cache *attachmentCache) save(attachment Attachment, content []byte) (string, error) {
	wanted := int64(len(content))
	if wanted > cache.limit {
		return "", fmt.Errorf("the attachment %s is %d bytes and the whole cache holds %d, so it cannot be kept", attachment.ID, wanted, cache.limit)
	}
	if err := cache.makeRoom(wanted); err != nil {
		return "", err
	}

	path := filepath.Join(cache.folder, cachedName(attachment))
	if err := os.WriteFile(path, content, contract.DataFileMode); err != nil {
		return "", fmt.Errorf("cannot write the attachment to %s, so check that the home folder is writable: %w", path, err)
	}
	return path, nil
}

// makeRoom throws out the files that have been in the cache longest until the
// wanted bytes will fit.
func (cache *attachmentCache) makeRoom(wanted int64) error {
	held, total, err := cache.contents()
	if err != nil {
		return err
	}
	sort.Slice(held, func(left, right int) bool { return held[left].when.Before(held[right].when) })

	for _, file := range held {
		if total+wanted <= cache.limit {
			return nil
		}
		if err := os.Remove(filepath.Join(cache.folder, file.name)); err != nil {
			return fmt.Errorf("cannot make room in the attachment cache by removing %s: %w", file.name, err)
		}
		total -= file.size
	}
	return nil
}

// cachedFile is one file in the cache, with what the ordering needs.
type cachedFile struct {
	name string
	size int64
	when time.Time
}

// contents lists what the cache holds and how much room it takes.
func (cache *attachmentCache) contents() ([]cachedFile, int64, error) {
	entries, err := os.ReadDir(cache.folder)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot read the attachment cache folder %s: %w", cache.folder, err)
	}

	held := make([]cachedFile, 0, len(entries))
	total := int64(0)
	for _, entry := range entries {
		if entry.IsDir() || len(held) >= maxCachedFiles {
			continue
		}
		about, err := entry.Info()
		if err != nil {
			continue
		}
		held = append(held, cachedFile{name: entry.Name(), size: about.Size(), when: about.ModTime()})
		total += about.Size()
	}
	return held, total, nil
}

// cachedName turns an attachment into a file name that is safe to write, keeping
// the ending so that a reader can tell what kind of file it is. The identifier
// the daemon gives is opaque and may hold anything at all, so every character
// that could mean something to a filesystem is replaced.
func cachedName(attachment Attachment) string {
	safe := strings.Map(keepPlainNameCharacters, attachment.ID)
	safe = strings.Trim(safe, ".")
	if safe == "" {
		safe = "attachment"
	}
	ending := strings.Map(keepPlainNameCharacters, filepath.Ext(attachment.Filename))
	if len(safe) > maxCachedNameLength {
		safe = safe[:maxCachedNameLength]
	}
	return safe + ending
}

// keepPlainNameCharacters keeps the letters, digits, dots, dashes and
// underscores of a name and drops everything else.
func keepPlainNameCharacters(letter rune) rune {
	switch {
	case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z', letter >= '0' && letter <= '9':
		return letter
	case letter == '.' || letter == '-' || letter == '_':
		return letter
	default:
		return -1
	}
}

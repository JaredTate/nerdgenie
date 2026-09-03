package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// DefaultAddress is where releases are published, and is what "coeus update"
// reads when it is given no address of its own. It is the download folder of
// the newest GitHub release, which serves the manifest, the sums file, and one
// archive per architecture under their own names.
const DefaultAddress = "https://github.com/JaredTate/coeus/releases/latest/download"

// The bounds on reading a release, because everything from outside the program
// is read under a cap.
const (
	// MaxDownloadBytes is the most one archive may be. The binary and the two
	// worker bundles together are tens of megabytes, so this is generous.
	MaxDownloadBytes = 256 << 20
	// manifestWait is how long the manifest request may take. It is a small
	// document, so a minute is already an unhealthy address.
	manifestWait = time.Minute
	// downloadWait is how long fetching one archive may take, which on a slow
	// line is minutes rather than seconds.
	downloadWait = 15 * time.Minute
	// maxRedirects is how many times a release address may move before the read
	// gives up, which is the number Go's own default policy stops at.
	maxRedirects = 10
)

// Source is where releases are read from: a web address beginning with http://
// or https://, or a folder on this machine, which is what a test and the
// installer's --from flag use.
type Source struct {
	// Address is the folder or web address holding the manifest and the
	// archives.
	Address string
	// MaxBytes caps one fetch, and is MaxDownloadBytes when it is zero.
	MaxBytes int64
	// Client is the client web addresses are read with, and is a client of this
	// package's own making when it is nil.
	Client *http.Client
}

// Manifest reads the release manifest the address serves.
func (source Source) Manifest(ctx context.Context) (Manifest, error) {
	written := &strings.Builder{}
	limited := source
	limited.MaxBytes = MaxManifestBytes
	if _, err := limited.fetchWithin(ctx, ManifestName, written, manifestWait); err != nil {
		return Manifest{}, err
	}
	return ParseManifest([]byte(written.String()))
}

// Fetch copies one file of the release into the writer and returns how many
// bytes it copied, refusing anything past the cap rather than filling the disk.
func (source Source) Fetch(ctx context.Context, name string, into io.Writer) (int64, error) {
	return source.fetchWithin(ctx, name, into, downloadWait)
}

// fetchWithin is Fetch with the time limit the caller needs: a minute for the
// manifest and a quarter of an hour for an archive.
func (source Source) fetchWithin(ctx context.Context, name string, into io.Writer, wait time.Duration) (int64, error) {
	if err := checkFetchName(name); err != nil {
		return 0, err
	}
	within, stop := context.WithTimeout(ctx, wait)
	defer stop()

	switch {
	case source.Address == "":
		return 0, errors.New("no release address was given, so there is nowhere to read a release from")
	case strings.HasPrefix(source.Address, "http://"), strings.HasPrefix(source.Address, "https://"):
		return source.fetchOverTheWeb(within, name, into)
	case strings.Contains(source.Address, "://"):
		return 0, fmt.Errorf("the release address %s is neither a web address nor a folder on this machine, so use an http address or a path",
			source.Address)
	default:
		return source.fetchFromAFolder(name, into)
	}
}

// checkFetchName refuses a file name that could climb out of the release.
func checkFetchName(name string) error {
	if name == "" || name != path.Clean(name) || path.IsAbs(name) || strings.HasPrefix(name, "..") {
		return fmt.Errorf("the file name %q is not one a release holds, so ask for a plain name such as %s", name, ManifestName)
	}
	return nil
}

// fetchFromAFolder copies one file out of a release folder on this machine.
func (source Source) fetchFromAFolder(name string, into io.Writer) (int64, error) {
	where := filepath.Join(source.Address, name)
	file, err := os.Open(where)
	if err != nil {
		return 0, fmt.Errorf("the release file %s could not be read, so check that the folder holds a release: %w", where, err)
	}
	defer func() { _ = file.Close() }()
	return source.copyWithin(name, into, file)
}

// fetchOverTheWeb copies one file of the release from a web address.
func (source Source) fetchOverTheWeb(ctx context.Context, name string, into io.Writer) (int64, error) {
	address := strings.TrimSuffix(source.Address, "/") + "/" + name
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, fmt.Errorf("the release address %s could not be asked for, so check that it is spelt right: %w", address, err)
	}
	answer, err := source.client().Do(request)
	if err != nil {
		return 0, fmt.Errorf("the release address %s could not be reached, so check the machine's network and try again: %w", address, err)
	}
	defer func() { _ = answer.Body.Close() }()

	if answer.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("the release address %s answered %d rather than serving %s, so check that the release is published",
			address, answer.StatusCode, name)
	}
	return source.copyWithin(name, into, answer.Body)
}

// copyWithin copies at most the cap and says so when there is more, because a
// file longer than the cap is not the release it claims to be.
func (source Source) copyWithin(name string, into io.Writer, from io.Reader) (int64, error) {
	limit := source.MaxBytes
	if limit <= 0 {
		limit = MaxDownloadBytes
	}
	copied, err := io.Copy(into, io.LimitReader(from, limit+1))
	if err != nil {
		return copied, fmt.Errorf("reading the release file %s stopped part way through, so try the update again: %w", name, err)
	}
	if copied > limit {
		return copied, fmt.Errorf("the release file %s is longer than the %d byte limit, so the address is serving something other than a release",
			name, limit)
	}
	return copied, nil
}

// client is the client a web address is read with, made here so that nothing
// outside this package has to know the timeouts or where a redirect may lead.
func (source Source) client() *http.Client {
	if source.Client != nil {
		return source.Client
	}
	return &http.Client{Timeout: downloadWait, CheckRedirect: refuseADowngrade}
}

// refuseADowngrade stops a release that was asked for over https from being
// followed to a plain address, and gives up on an address that keeps moving.
//
// The manifest is what carries the checksums, so a plaintext manifest and a
// plaintext archive that agrees with it are a whole release nobody has checked;
// whoever can answer in the middle of a plain connection can write both.
func refuseADowngrade(request *http.Request, sent []*http.Request) error {
	if len(sent) >= maxRedirects {
		return fmt.Errorf("the release address moved %d times without settling, which is more than the %d this reads, so name the address it ends at with --from",
			len(sent), maxRedirects)
	}
	if len(sent) > 0 && sent[0].URL.Scheme == "https" && request.URL.Scheme != "https" {
		return fmt.Errorf("the release address %s sent Coeus on to %s, which is not https, so nothing was read; a release is only as trustworthy as the manifest that names its checksums, so name an https address with --from",
			sent[0].URL, request.URL)
	}
	return nil
}

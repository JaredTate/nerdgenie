package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The bounds on one fetch.
const (
	// MaxPageBytes is how much of one page is read and turned into text.
	MaxPageBytes = 200 << 10
	// MaxRedirects is how many times a fetch follows a page that points
	// somewhere else before it gives up.
	MaxRedirects = 5
	// userAgent is what the agent calls itself when it asks for a page, because
	// a request with no name on it is a request nobody can trace.
	userAgent = "nerdgenie/1 (an agent fetching a page a person asked for)"
)

// fetched is one page, as it came back.
type fetched struct {
	address     string
	contentType string
	body        string
}

// fetchPage asks for a page at an address the agent is allowed to reach, follows
// the pages that point somewhere else while each of those is allowed too, and
// returns what came back, inside the size cap. Every hop goes through the guard
// before it is asked for, because a page that points somewhere else is choosing
// where the agent goes next, and the first address being allowed says nothing
// about the fifth.
func (tool *Tool) fetchPage(ctx context.Context, address string) (fetched, error) {
	at, cameFrom := address, ""
	for hop := 0; hop <= MaxRedirects; hop++ {
		pinned, err := PinnedAddress(at, tool.settings.AllowedHosts)
		if err != nil {
			return fetched{}, refusedHop(cameFrom, at, err)
		}
		answer, err := tool.askFor(ctx, at, pinned)
		if err != nil {
			return fetched{}, err
		}
		next, isRedirect := redirectFrom(answer, at)
		if !isRedirect {
			return readAnswer(answer, at)
		}
		_ = answer.Body.Close()
		cameFrom, at = at, next
	}
	return fetched{}, fmt.Errorf("the address %s went round more than %d redirects, so it never settled on a page", address, MaxRedirects)
}

// refusedHop says why a hop was refused, naming the page that sent the agent
// there when it was a page rather than the person who asked. A refusal that gave
// only the number would leave the person wondering how the agent came to be
// asking for an address nobody typed.
func refusedHop(cameFrom string, at string, err error) error {
	if cameFrom == "" {
		return err
	}
	return fmt.Errorf("%s redirected the agent to %s, which it may not reach: %w", cameFrom, at, err)
}

// askFor makes one request, connecting to the exact number the name resolved to
// rather than resolving it again.
func (tool *Tool) askFor(ctx context.Context, address string, pinned string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot ask for %s: %w", address, err)
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.1")

	client := &http.Client{
		Timeout: tool.timeout(),
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, _ string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: tool.timeout()}
				return dialer.DialContext(ctx, network, pinned)
			},
			TLSHandshakeTimeout:   tool.timeout(),
			ResponseHeaderTimeout: tool.timeout(),
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	answer, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the address %s: %w", address, err)
	}
	return answer, nil
}

// redirectFrom says whether an answer points somewhere else, and where.
func redirectFrom(answer *http.Response, at string) (string, bool) {
	if answer.StatusCode < 300 || answer.StatusCode > 399 {
		return "", false
	}
	pointed := answer.Header.Get("Location")
	if pointed == "" {
		return "", false
	}
	here, err := url.Parse(at)
	if err != nil {
		return pointed, true
	}
	next, err := url.Parse(pointed)
	if err != nil {
		return pointed, true
	}
	return here.ResolveReference(next).String(), true
}

// readAnswer reads what came back, refusing anything that is not text and
// stopping at the size cap.
func readAnswer(answer *http.Response, address string) (fetched, error) {
	defer func() { _ = answer.Body.Close() }()

	if answer.StatusCode < 200 || answer.StatusCode > 299 {
		return fetched{}, fmt.Errorf("the address %s answered %d %s, so check the address",
			address, answer.StatusCode, http.StatusText(answer.StatusCode))
	}
	contentType := answer.Header.Get("Content-Type")
	if !isText(contentType) {
		return fetched{}, fmt.Errorf("the address %s holds %s, which is not text, so this tool cannot read it",
			address, strings.TrimSpace(contentType))
	}
	held, err := io.ReadAll(io.LimitReader(answer.Body, MaxPageBytes))
	if err != nil {
		return fetched{}, fmt.Errorf("cannot read what %s answered with: %w", address, err)
	}
	return fetched{address: address, contentType: contentType, body: string(held)}, nil
}

// isText says whether a kind of content is something this tool can read. An
// answer with no kind at all is taken to be text, because a great many servers
// send none.
func isText(contentType string) bool {
	kind := strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	switch {
	case kind == "":
		return true
	case strings.HasPrefix(kind, "text/"):
		return true
	case kind == "application/json" || kind == "application/xml" || kind == "application/xhtml+xml":
		return true
	default:
		return false
	}
}

// timeout is how long one request has, with half a minute when the settings name
// nothing, because every outside call has to end.
func (tool *Tool) timeout() time.Duration {
	if tool.settings.Timeout > 0 {
		return tool.settings.Timeout
	}
	return 30 * time.Second
}

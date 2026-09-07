package shell

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// CheckTimeout is how long a check waits for a port to answer. A server on
// this machine answers in milliseconds; two seconds is long enough for one
// that is busy and short enough that a dead one costs no round.
const CheckTimeout = 2 * time.Second

// check says whether a port on this machine answers, and how fast, with no
// shell in it: one task ran the same curl health probe twenty-six times, a
// round each. A path asks for it over HTTP and reports the status; no path
// opens a connection and closes it.
func (tool *Tool) check(ctx context.Context, asked Call) (contract.ToolOutput, error) {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(asked.Port))
	began := time.Now()
	if strings.TrimSpace(asked.Path) != "" {
		return checkOverHTTP(ctx, asked, address, began)
	}
	connection, err := (&net.Dialer{Timeout: CheckTimeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return contract.ToolOutput{Text: fmt.Sprintf("nothing listens on %d\n", asked.Port)}, nil
	}
	_ = connection.Close()
	return contract.ToolOutput{Text: fmt.Sprintf("port %d answers: a connection opened in %d ms\n", asked.Port, millisecondsSince(began))}, nil
}

// checkOverHTTP fetches the path from the port and reports the status code
// and the time, or says nothing listens when the connection is refused.
func checkOverHTTP(ctx context.Context, asked Call, address string, began time.Time) (contract.ToolOutput, error) {
	path := asked.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+path, nil)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the path %q cannot be fetched, so give a path such as /index.html: %w", asked.Path, err)
	}
	client := &http.Client{Timeout: CheckTimeout}
	response, err := client.Do(request)
	if err != nil {
		return contract.ToolOutput{Text: fmt.Sprintf("nothing listens on %d\n", asked.Port)}, nil
	}
	_ = response.Body.Close()
	return contract.ToolOutput{Text: fmt.Sprintf("port %d answers: HTTP %d in %d ms\n", asked.Port, response.StatusCode, millisecondsSince(began))}, nil
}

// millisecondsSince is the whole milliseconds since the moment.
func millisecondsSince(began time.Time) int64 {
	return time.Since(began).Milliseconds()
}

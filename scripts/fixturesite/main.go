// Command fixturesite serves the fixture web site from test/fixtures/site on a
// loopback port, so a person can watch the agent sign in, post with a preview,
// and stop at the captcha page during the human trial. It prints the address
// and the one credential the login page accepts, and serves until it is
// interrupted.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

// readHeaderWait bounds how long a slow visitor may take to send its headers.
const readHeaderWait = 10 * time.Second

func main() {
	listen := flag.String("listen", "127.0.0.1:8471", "the address to serve on")
	flag.Parse()
	if err := run(context.Background(), *listen, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fixturesite:", err)
		os.Exit(1)
	}
}

// run serves the site on the address until the context ends or the process is
// interrupted, and says where it is and what it accepts.
func run(ctx context.Context, listen string, output *os.File) error {
	site, err := testkit.NewFixtureSite(testkit.FixturePagesFolder())
	if err != nil {
		return fmt.Errorf("the pages could not be read, so run this from the repository: %w", err)
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", listen, err)
	}
	fmt.Fprintf(output, "the fixture site is at http://%s/login\n", listener.Addr())
	fmt.Fprintf(output, "it accepts username %q, password %q, and code %q; the captcha page is /captcha\n",
		testkit.FixtureUsername, testkit.FixturePassword, testkit.FixtureCode)

	server := &http.Server{Handler: site.Handler(), ReadHeaderTimeout: readHeaderWait}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

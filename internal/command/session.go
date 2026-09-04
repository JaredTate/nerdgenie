package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// sessionTimeFormat is how a session's start time is printed: the date and the
// time of day, with no time zone, because the listing is read on the machine
// that wrote it.
const sessionTimeFormat = "2006-01-02 15:04"

// NewSession is the "/new" command: it starts a fresh conversation and leaves
// everything already running where it is.
func (commands *Commands) NewSession() contract.Command {
	return contract.Command{
		Name: "new",
		Help: "Starts a fresh session, leaving the ones already there alone.",
		Run: func(ctx context.Context, _ string, _ contract.CommandContext) (string, error) {
			if commands.deps.NewSession == nil {
				return "", notWiredUp("new", "NewSession")
			}
			started, err := commands.deps.NewSession(ctx)
			if err != nil {
				return "", fmt.Errorf("a fresh session could not be started: %w", err)
			}
			return fmt.Sprintf("started session %s. The ones you were in are still there; type /sessions to see them.", started), nil
		},
	}
}

// Sessions is the "/sessions" command: it lists the conversations, newest
// first, and switches to one of them by its id.
func (commands *Commands) Sessions() contract.Command {
	return contract.Command{
		Name: "sessions",
		Help: "Lists the sessions, newest first, and switches to one of them: /sessions 11.",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			if commands.deps.Sessions == nil {
				return "", notWiredUp("sessions", "Sessions")
			}
			listed, err := commands.deps.Sessions(ctx)
			if err != nil {
				return "", fmt.Errorf("the sessions could not be listed: %w", err)
			}
			if wanted := strings.TrimSpace(arguments); wanted != "" {
				return commands.switchSession(ctx, listed, wanted)
			}
			return sessionListing(listed), nil
		},
	}
}

// switchSession makes one of the listed sessions the one the user is talking
// in, refusing an id the listing does not hold so that a typo cannot start a
// conversation that is not there.
func (commands *Commands) switchSession(ctx context.Context, listed []Session, wanted string) (string, error) {
	held := false
	for _, session := range listed {
		if session.ID == wanted {
			held = true
			break
		}
	}
	if !held {
		return fmt.Sprintf("there is no session called %q, so type /sessions for the list.", wanted), nil
	}
	if commands.deps.SwitchSession == nil {
		return "", notWiredUp("sessions", "SwitchSession")
	}
	if err := commands.deps.SwitchSession(ctx, wanted); err != nil {
		return "", fmt.Errorf("the session %s could not be switched to: %w", wanted, err)
	}
	return fmt.Sprintf("you are now in session %s.", wanted), nil
}

// sessionListing is the text "/sessions" answers with: one line per session,
// with the one the user is in marked, and the line saying how to switch.
func sessionListing(listed []Session) string {
	if len(listed) == 0 {
		return "there are no sessions yet, so say something and Coeus will start one.\n"
	}

	widest := 0
	for _, session := range listed {
		if len(session.ID) > widest {
			widest = len(session.ID)
		}
	}

	written := &strings.Builder{}
	written.WriteString("these are the sessions, newest first:\n\n")
	for _, session := range listed {
		fmt.Fprintf(written, "  %-*s  %s  %s", widest, session.ID, session.Started.Format(sessionTimeFormat), session.Title)
		if session.Current {
			written.WriteString("  (this one)")
		}
		written.WriteString("\n")
	}
	fmt.Fprintf(written, "\nType /sessions %s to switch to one of them.\n", exampleSessionID(listed))
	return written.String()
}

// exampleSessionID is the id the closing line uses as its example: the first
// session the user is not already in, so that following the line does
// something.
func exampleSessionID(listed []Session) string {
	for _, session := range listed {
		if !session.Current {
			return session.ID
		}
	}
	return listed[0].ID
}

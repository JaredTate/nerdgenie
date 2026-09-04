package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// yoloName is the slash command that turns the asking off for a session.
const yoloName = "yolo"

// The two answers the yolo command gives, one for each way the switch can be.
const (
	theYoloOnText  = "yolo is on: every call that would have stopped for a yes now runs without asking, and is logged as allowed by yolo. A rule that says never is still never. Type /yolo off to bring the asking back; a restart brings it back too."
	theYoloOffText = "yolo is off: a call on your ask-me-first list, or one the harness cannot read to the end, asks first again."
)

// yoloCommand is the "/yolo" command: on its own, or with the word on, it turns
// yolo on for the rest of the session and says so; with the word off, it turns
// yolo off and says so. The switch lives in the permission function, which is
// the one place that decides whether a call asks, the way "/think" hands its
// level to the loop, which is the one place that makes a call. Nothing is
// written to disk, so a restart starts with it off.
func (running *agent) yoloCommand() contract.Command {
	return contract.Command{
		Name: yoloName,
		Help: "Runs every call that would have asked first without asking, for this session; /yolo off brings the asking back.",
		Run: func(_ context.Context, arguments string, _ contract.CommandContext) (string, error) {
			switch strings.ToLower(strings.TrimSpace(arguments)) {
			case "", "on":
				running.decider.UseYolo(true)
				return theYoloOnText, nil
			case "off":
				running.decider.UseYolo(false)
				return theYoloOffText, nil
			default:
				return "", fmt.Errorf("/yolo takes nothing, on, or off, and %q is none of those", strings.TrimSpace(arguments))
			}
		},
	}
}

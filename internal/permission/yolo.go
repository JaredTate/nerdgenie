package permission

import (
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// AllowedByYolo is how every ruling made under yolo begins, so that anybody
// reading the event log can find the calls that ran because the switch was on.
const AllowedByYolo = "allowed by yolo"

// UseYolo turns yolo on or off for the rest of the session, which is what the
// "/yolo" command does. While it is on, every call that would have stopped for
// a yes runs without asking and is logged as allowed by yolo. It is kept in
// memory only, so a restart starts with it off, and it changes nothing about a
// rule that refuses or a no the user gave this session, because those are the
// user's own word and the switch only stands in for the yes.
func (decider *Decider) UseYolo(on bool) {
	decider.guard.Lock()
	defer decider.guard.Unlock()
	decider.yolo = on
}

// YoloIsOn says whether yolo is on, which "/yolo" and "/status" both report.
func (decider *Decider) YoloIsOn() bool {
	decider.guard.Lock()
	defer decider.guard.Unlock()
	return decider.yolo
}

// allowedByYolo is the ruling a call gets when it would have asked and yolo is
// on: an allow whose reason says so and says what the harness would have asked
// about, carrying the same preview an approved call carries, so that the log
// holds exactly what ran.
func allowedByYolo(request contract.PermissionRequest, reduced string, why string) contract.PermissionDecision {
	return contract.PermissionDecision{
		Ruling:      contract.RulingAllow,
		Reason:      fmt.Sprintf("%s, which you switched on for this session; without it this would have asked first: %s", AllowedByYolo, why),
		PreviewText: previewOf(request, reduced),
	}
}

package main

import (
	"testing"
)

func TestStreamingThePiecesDoesNothingWithNoSocket(t *testing.T) {
	// With no socket open there is nowhere for the pieces to go, and neither
	// call must panic reaching for one that is not there.
	running := &agent{}
	running.streamReplyPiece("half a word")
	running.withdrawStreamedReply()
}

func TestStreamingThePiecesReachesTheSocketWhenThereIsOne(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// No screen is attached, so both are quiet successes; what matters is that
	// the loop's two seams run against a real socket without trouble.
	running.streamReplyPiece("here is ")
	running.streamReplyPiece("the answer")
	running.withdrawStreamedReply()
}

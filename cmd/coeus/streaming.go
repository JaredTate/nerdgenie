package main

import "context"

// The reply's pieces travel from the model to the screens through the socket as
// they are written, and a retry withdraws them (brief 6.8). These two are the
// loop's and the provider's seams for that.

// streamReplyPiece hands the screens one piece of the reply the model is
// writing. A piece that cannot be sent is dropped, because the finished reply
// carries the whole text and is the authority over the pieces.
func (running *agent) streamReplyPiece(piece string) {
	if running.socket == nil {
		return
	}
	_ = running.socket.SendDelta(context.Background(), piece)
}

// withdrawStreamedReply tells the screens to take the partial reply down,
// because the call behind it failed and is being tried again.
func (running *agent) withdrawStreamedReply() {
	if running.socket == nil {
		return
	}
	_ = running.socket.ResetDelta(context.Background())
}

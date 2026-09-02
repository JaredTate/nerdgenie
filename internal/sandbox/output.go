package sandbox

import "fmt"

// cappedWriter keeps the first so many bytes written to it and counts the rest.
// A command that prints without stopping must not be able to fill this program's
// memory, and a model that is handed a hundred megabytes of build log has learnt
// nothing that the first thirty thousand bytes did not already tell it.
type cappedWriter struct {
	limit   int
	kept    []byte
	dropped int
}

// Write keeps what still fits and counts what does not. It always reports the
// whole chunk as written, because a command that is told its output was short
// written will usually stop, and the point is to read it to the end and throw
// the middle away.
func (writer *cappedWriter) Write(chunk []byte) (int, error) {
	whole := len(chunk)
	if room := writer.limit - len(writer.kept); room > 0 {
		taken := min(room, len(chunk))
		writer.kept = append(writer.kept, chunk[:taken]...)
		chunk = chunk[taken:]
	}
	writer.dropped += len(chunk)
	return whole, nil
}

// Bytes is what was kept, with one line at the end when anything was dropped, so
// that whoever reads the output knows there was more of it.
func (writer *cappedWriter) Bytes() []byte {
	if writer.dropped == 0 {
		return writer.kept
	}
	note := fmt.Sprintf("\n[coeus dropped %d more bytes here, because the output passed the cap of %d bytes.]\n",
		writer.dropped, writer.limit)
	return append(append([]byte{}, writer.kept...), note...)
}

package permission

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxPreviewRunes is the cap on a preview. A preview is read by a person before
// they answer, and a page of text is not read, it is skipped.
const MaxPreviewRunes = 4000

// previewOf returns exactly what is about to happen, in plain words, the way a
// person reads a text message back before hitting send. Unlike the readable
// form, a preview may run to several lines, because it is shown and not matched.
func previewOf(request contract.PermissionRequest, reduced string) string {
	fields := readFields(request.Input)
	switch request.ToolName {
	case contract.ToolShell:
		return cutToPreviewCap(shellPreview(request, fields))
	case contract.ToolWrite:
		return cutToPreviewCap(writePreview(fields, reduced))
	case contract.ToolEdit:
		return cutToPreviewCap(editPreview(fields, reduced))
	case contract.ToolWeb:
		return cutToPreviewCap(webPreview(fields, reduced))
	default:
		return cutToPreviewCap(actionPreview(request.ToolName, fields, reduced))
	}
}

// shellPreview shows the whole command, not the readable form, because the
// argument the readable form leaves out is exactly the part a person checks.
func shellPreview(request contract.PermissionRequest, fields map[string]json.RawMessage) string {
	command, written := stringField(fields, "command")
	if !written || strings.TrimSpace(command) == "" {
		command = request.CommandPrefix
	}
	if !boolField(fields, "escalate") {
		return "run this command:\n\n" + command
	}
	why, given := stringField(fields, "reason")
	if given && strings.TrimSpace(why) != "" {
		return fmt.Sprintf("run this command with administrator powers, because %s:\n\n%s", strings.TrimSpace(why), command)
	}
	return "run this command with administrator powers:\n\n" + command
}

// writePreview shows the file and how much is about to be written to it, and
// says plainly when what is about to be written is nothing at all.
func writePreview(fields map[string]json.RawMessage, reduced string) string {
	path, written := stringField(fields, "path")
	if !written || path == "" {
		return reduced
	}
	content, _ := stringField(fields, "content")
	if strings.TrimSpace(content) != "" {
		return fmt.Sprintf("write %d bytes to the file %s", len(content), path)
	}
	if size := sizeOnDisk(path); size > 0 {
		return fmt.Sprintf("empty the file %s, which holds %d bytes now", path, size)
	}
	return "write an empty file at " + path
}

// editPreview shows the file and how much of it changes.
func editPreview(fields map[string]json.RawMessage, reduced string) string {
	path, written := stringField(fields, "path")
	if !written || path == "" {
		return reduced
	}
	removed, _ := stringField(fields, "old")
	replacement, _ := stringField(fields, "new")
	return fmt.Sprintf("in the file %s, replace %d bytes with %d bytes", path, len(removed), len(replacement))
}

// webPreview shows the address about to be fetched, or the words about to be
// searched for.
func webPreview(fields map[string]json.RawMessage, reduced string) string {
	if address, written := stringField(fields, "url"); written && strings.TrimSpace(address) != "" {
		return "fetch the page " + strings.TrimSpace(address)
	}
	if words, written := stringField(fields, "query"); written && strings.TrimSpace(words) != "" {
		return "search the web for " + strings.TrimSpace(words)
	}
	return reduced
}

// actionPreview shows what a browser or desktop call means to do and which
// element on the page or the screen it means to do it to.
func actionPreview(toolName string, fields map[string]json.RawMessage, reduced string) string {
	intent, written := stringField(fields, "intent")
	if !written || strings.TrimSpace(intent) == "" {
		return reduced
	}
	line := toolName + ": " + strings.TrimSpace(intent)
	if element, named := stringField(fields, "element"); named && strings.TrimSpace(element) != "" {
		line += ", on the element " + strings.TrimSpace(element)
	}
	return line
}

// cutToPreviewCap keeps a preview inside the cap.
func cutToPreviewCap(preview string) string {
	letters := []rune(preview)
	if len(letters) > MaxPreviewRunes {
		return string(letters[:MaxPreviewRunes])
	}
	return preview
}

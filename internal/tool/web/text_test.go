package web_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/web"
)

func TestHeadingsLinksAndListsSurviveTheTurnIntoText(t *testing.T) {
	page := `<html><head><title>Notes</title><style>body{color:red}</style></head><body>
<h1>The heading</h1>
<p>A paragraph with a <a href="https://example.com/one">link in it</a>.</p>
<ul><li>the first item</li><li>the second item</li></ul>
<script>console.log("nothing to see")</script>
<h2>A smaller heading</h2>
<p>Ampersands &amp; angle brackets &lt;like this&gt; come back as themselves.</p>
</body></html>`

	testkit.Golden(t, "a_page_as_text.txt", []byte(web.HTMLToText(page)))
}

func TestTheWordsInsideAScriptAreNotInTheText(t *testing.T) {
	text := web.HTMLToText(`<html><body><script>alert("do this")</script><p>the real words</p></body></html>`)

	if strings.Contains(text, "do this") {
		t.Errorf("the words inside a script are in the text: %q", text)
	}
	if !strings.Contains(text, "the real words") {
		t.Errorf("the words of the page are not in the text: %q", text)
	}
}

func TestTextWithNoTagsAtAllComesBackAsItself(t *testing.T) {
	text := web.HTMLToText("just a line of text\nand another\n")

	if !strings.Contains(text, "just a line of text") {
		t.Errorf("text with no tags came back as %q", text)
	}
}

func TestTheTextOfAPageIsBounded(t *testing.T) {
	page := "<p>" + strings.Repeat("word ", web.MaxPageBytes) + "</p>"

	if length := len(web.HTMLToText(page)); length > web.MaxPageBytes {
		t.Errorf("the text of a very long page is %d bytes, and the cap is %d", length, web.MaxPageBytes)
	}
}

func TestATagWhoseNameOrAttributesAreWrittenInCapitalsStillReads(t *testing.T) {
	text := web.HTMLToText(`<A HREF="https://example.com/x">a link</A>`)

	if !strings.Contains(text, "https://example.com/x") {
		t.Errorf("a tag written in capitals came back as %q", text)
	}
}

func TestALetterThatChangesLengthWhenItIsMadeSmallDoesNotUpsetTheAttributeReader(t *testing.T) {
	// The Kelvin sign takes three bytes as it is written and one when it is made
	// small, so a reader that looks for an attribute in a lowercased copy and
	// then cuts the original at what it found would cut in the wrong place. The
	// fuzzer found this, and the input it found it with is in testdata.
	for _, page := range []string{"<A \u212ahref=>", "<a \u212ahref=\"https://example.com\">x</a>", "<A \xc4href=>"} {
		if text := web.HTMLToText(page); strings.Contains(text, "<") {
			t.Errorf("the page %q came back with a tag still in it: %q", page, text)
		}
	}
}

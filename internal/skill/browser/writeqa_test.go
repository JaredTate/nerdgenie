package browser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill/browser"
)

// TestWriteTheShippedQualitySkill is how skills/qa is generated. It is skipped
// unless a person asks for it by hand with
// "NERDGENIE_WRITE_QA_SKILL=1 go test ./internal/skill/browser -run TestWriteTheShippedQualitySkill",
// because the folder it writes is checked in and the test beside it is what
// keeps the two the same.
func TestWriteTheShippedQualitySkill(t *testing.T) {
	if !askedToWriteTheSkill() {
		t.Skip("the shipped skill folder is only rewritten when NERDGENIE_WRITE_QA_SKILL is set to 1")
	}
	folder := filepath.Join("..", "..", "..", "skills", browser.QASkillName)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder %s: %v", folder, err)
	}
	for name, content := range browser.ShippedQASkill() {
		if err := os.WriteFile(filepath.Join(folder, name), content, contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}
}

// askedToWriteTheSkill says whether the person running the tests asked for the
// shipped folder to be rewritten.
func askedToWriteTheSkill() bool {
	return os.Getenv("NERDGENIE_WRITE_QA_SKILL") == "1"
}

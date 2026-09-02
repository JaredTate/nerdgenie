package sandbox

import (
	"bytes"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTheHandledAccessMatchesWhatEachLandlockVersionKnowsAbout(t *testing.T) {
	cases := []struct {
		version int
		want    uint64
	}{
		{version: 0, want: 0x1fff},
		{version: 1, want: 0x1fff},
		{version: 2, want: 0x3fff},
		{version: 3, want: 0x7fff},
		{version: 4, want: 0x7fff},
		{version: 5, want: 0xffff},
		{version: 7, want: 0xffff},
		{version: 99, want: 0xffff},
	}

	for _, want := range cases {
		if got := handledAccess(want.version); got != want.want {
			t.Errorf("landlock version %d handles %#x, want %#x", want.version, got, want.want)
		}
	}
}

func TestTheReadableAccessIsReadAndRunAndNothingElse(t *testing.T) {
	var wanted uint64 = accessExecute | accessReadFile | accessReadDirectory

	if readableAccess() != wanted {
		t.Errorf("a read-only folder is given %#x, want %#x, which is read and run and nothing else", readableAccess(), wanted)
	}
	for _, version := range []int{1, 3, 5, 7} {
		if readableAccess()&^handledAccess(version) != 0 {
			t.Errorf("a read-only folder is given rights that landlock version %d does not know about", version)
		}
	}
}

func TestTheWritableAccessIsEverythingTheVersionKnowsAbout(t *testing.T) {
	for _, version := range []int{1, 2, 3, 5, 7} {
		if writableAccess(version) != handledAccess(version) {
			t.Errorf("a sandbox root is given %#x on landlock version %d, want everything the ruleset arbitrates, %#x",
				writableAccess(version), version, handledAccess(version))
		}
	}
}

func TestARuleOnAFileIsCutDownToTheRightsAFileCanHave(t *testing.T) {
	full := writableAccess(7)

	if accessForTarget(full, true) != full {
		t.Errorf("a rule on a folder was cut down to %#x, want everything at %#x", accessForTarget(full, true), full)
	}
	onAFile := accessForTarget(full, false)
	if onAFile&accessReadDirectory != 0 || onAFile&accessMakeRegularFile != 0 {
		t.Errorf("a rule on a file keeps %#x, and a file cannot be listed or have files made inside it", onAFile)
	}
	if onAFile&accessExecute == 0 || onAFile&accessReadFile == 0 {
		t.Errorf("a rule on a file keeps %#x, and the helper program has to be readable and runnable", onAFile)
	}
}

func TestTheKernelRefusesARulesetAskingForRightsItHasNeverHeardOf(t *testing.T) {
	if _, err := createLandlockRuleset(1 << 40); err == nil {
		t.Fatal("the kernel accepted a ruleset arbitrating a right that does not exist")
	}
}

func TestARuleCannotBeAddedToSomethingThatIsNotARuleset(t *testing.T) {
	if err := addLandlockRule(-1, "/usr", readableAccess()); err == nil {
		t.Fatal("a rule was added to a file number that is not a ruleset")
	}
}

func TestTheRulesetAttributeIsEightBytesInTheOrderTheKernelReads(t *testing.T) {
	encoded := encodeRulesetAttribute(0x1fff)

	want := []byte{0xff, 0x1f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(encoded[:], want) {
		t.Errorf("the ruleset attribute is %#v, want %#v", encoded, want)
	}
}

func TestThePathRuleIsTwelveBytesWithNoPaddingInTheMiddle(t *testing.T) {
	encoded := encodePathBeneathRule(0x8000, 7)

	want := []byte{0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00}
	if len(encoded) != pathBeneathRuleSize {
		t.Fatalf("the rule is %d bytes, want %d, because the kernel's struct is packed", len(encoded), pathBeneathRuleSize)
	}
	if !bytes.Equal(encoded[:], want) {
		t.Errorf("the rule is %#v, want %#v", encoded, want)
	}
}

func TestTheKernelReportsALandlockVersionOnThisMachine(t *testing.T) {
	version, err := landlockVersion()
	if err != nil {
		t.Fatalf("the kernel does not report a Landlock version, and %s lists landlock: %v", securityModulesFile, err)
	}
	if version < 1 {
		t.Errorf("the kernel reports Landlock version %d, want one or more", version)
	}
}

func TestARulesetIsBuiltFromTheFoldersTheHelperWasGiven(t *testing.T) {
	work := t.TempDir()

	rulesetFile, version, err := buildLandlockRuleset([]string{"/usr"}, []string{work})
	if err != nil {
		t.Fatalf("cannot build a ruleset from real folders: %v", err)
	}
	defer func() {
		if err := syscall.Close(rulesetFile); err != nil {
			t.Errorf("cannot close the ruleset: %v", err)
		}
	}()

	if rulesetFile < 0 {
		t.Errorf("the ruleset came back as file %d, want a real one", rulesetFile)
	}
	if version < 1 {
		t.Errorf("the ruleset was built against Landlock version %d, want one or more", version)
	}
}

func TestARulesetSkipsAFolderThatIsNotOnThisMachine(t *testing.T) {
	work := t.TempDir()
	missing := filepath.Join(work, "no-such-folder")

	rulesetFile, _, err := buildLandlockRuleset([]string{"/usr", missing}, []string{work})
	if err != nil {
		t.Fatalf("a folder that is not there stopped the ruleset instead of being skipped: %v", err)
	}
	if err := syscall.Close(rulesetFile); err != nil {
		t.Errorf("cannot close the ruleset: %v", err)
	}
}

func TestARulesetRefusesAFolderNameHoldingAZeroByte(t *testing.T) {
	work := t.TempDir()

	if _, _, err := buildLandlockRuleset([]string{"/usr\x00/etc"}, []string{work}); err == nil {
		t.Fatal("a folder name holding a zero byte was accepted, and a zero byte hides whatever follows it")
	}
}

package skill_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theSkillAModelWrote is the folder a model hands the skill tool: a permissions
// block naming one website, one step that visits it, and a source line of its
// own saying a person saved it, which is the lie the store has to write over.
func theSkillAModelWrote() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# read-the-story\n\nReads today's story off the news site.\n\n" +
			"## Permissions\n\n- source: person\n- site: news.example.com\n- daily limit: 50\n"),
		skill.StepsFile: []byte("1. Read today's story.\n" +
			"   tool: web\n   input: {\"url\": \"https://news.example.com/story\"}\n"),
	}
}

func TestASkillTheModelSavedSaysSoInItsFrontMatter(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByModel, "read-the-story", theSkillAModelWrote()); err != nil {
		t.Fatalf("saving the skill the model wrote failed: %v", err)
	}

	written := built.readSkillFile(t, "read-the-story", skill.DescriptionFile)
	if !strings.Contains(written, "- source: model") {
		t.Errorf("the saved %s is %q, want it to say the model saved this skill, because the person reading it has to be able to see that", skill.DescriptionFile, written)
	}
	if strings.Contains(written, "- source: person") {
		t.Errorf("the saved %s is %q, and it still holds the source line the model wrote; whoever is saving is the store's word, not the file's", skill.DescriptionFile, written)
	}
}

func TestASkillAPersonSavedCarriesNoSourceLineAtAll(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "read-the-story", theSkillAModelWrote()); err != nil {
		t.Fatalf("saving the skill the person saved failed: %v", err)
	}

	written := built.readSkillFile(t, "read-the-story", skill.DescriptionFile)
	if strings.Contains(written, "source:") {
		t.Errorf("the saved %s is %q, want no source line, because the mark is there to say a skill was not a person's", skill.DescriptionFile, written)
	}
}

func TestASaveWithNobodySavingItIsRefused(t *testing.T) {
	built := newHarness(t)
	err := built.store.Save(context.Background(), contract.SkillSource("nobody"), "read-the-story", theSkillAModelWrote())
	if err == nil {
		t.Fatal("a save that named nobody as its source was written; the store cannot mark a skill it does not know the source of")
	}
	if !strings.Contains(err.Error(), "person") || !strings.Contains(err.Error(), "model") {
		t.Errorf("the message is %q, and it has to name the two sources a save may have", err)
	}
}

func TestASkillFolderCannotHandInThePersonsApproval(t *testing.T) {
	built := newHarness(t)
	files := theSkillAModelWrote()
	files[skill.ApprovedByPersonFile] = []byte("2026-09-02\n")

	err := built.store.Save(context.Background(), contract.SkillSavedByModel, "read-the-story", files)
	if err == nil {
		t.Fatal("a folder holding the store's own record that a person approved it was saved; the model could then write itself" +
			" the very yes that lets a skill it wrote hold a standing approval")
	}
	if !strings.Contains(err.Error(), skill.ApprovedByPersonFile) {
		t.Errorf("the message is %q, and it has to name the file a skill folder may not hold", err)
	}
}

func TestASkillTheModelSavedGetsNoStandingApprovalUntilAPersonHasRunIt(t *testing.T) {
	fetched := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb},
		"today's story", "today's story", "today's story", "today's story")
	store, channel, _ := realPermissionHarnessWith(t, configurationWhereEveryFetchAsks(), fetched)
	ctx := context.Background()
	if err := store.Save(ctx, contract.SkillSavedByModel, "read-the-story", theSkillAModelWrote()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	// The model runs it twice through the tool's path. Every step asks, because
	// a skill the model wrote holds no standing approval, and a run the model
	// started is never the person's yes however the person answers it.
	channel.AnswerPreviewsWith(contract.AnswerOnce)
	for round := range 2 {
		if _, err := store.Run(ctx, "read-the-story", ""); err != nil {
			t.Fatalf("run %d failed: %v", round+1, err)
		}
	}
	if previews := channel.Previews(); len(previews) != 2 {
		t.Fatalf("the person was asked %d times over two runs the model started, want once for each,"+
			" because a skill the model wrote holds no standing approval", len(previews))
	}

	// The person runs it themselves and says yes, which is what a skill the
	// model wrote needs before its permissions block means anything.
	if _, err := store.RunForPerson(ctx, "read-the-story", ""); err != nil {
		t.Fatalf("the run the person started failed: %v", err)
	}
	if previews := channel.Previews(); len(previews) != 3 {
		t.Fatalf("the person was asked %d times, want three, the third being their own run", len(previews))
	}

	if _, err := store.Run(ctx, "read-the-story", ""); err != nil {
		t.Fatalf("the run after the person said yes failed: %v", err)
	}
	if previews := channel.Previews(); len(previews) != 3 {
		t.Errorf("the person was asked %d times, want three: once a person has run the skill and said yes,"+
			" its permissions block becomes standing approvals like any other skill's", len(previews))
	}
}

func TestThePersonsYesIsRememberedBesideTheSkillAndGoesWhenItIsSavedOver(t *testing.T) {
	built := newHarness(t, testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "today's story", "today's story"))
	built.permission.Rule(contract.ToolWeb, contract.PermissionDecision{Ruling: contract.RulingAsk, Reason: "the person asked to see every page fetched"})
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByModel, "read-the-story", theSkillAModelWrote()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	built.channel.AnswerPreviewsWith(contract.AnswerOnce)
	if _, err := built.store.RunForPerson(ctx, "read-the-story", ""); err != nil {
		t.Fatalf("the run the person started failed: %v", err)
	}
	remembered := filepath.Join(built.home.SkillFolder("read-the-story"), skill.ApprovedByPersonFile)
	if _, err := os.Stat(remembered); err != nil {
		t.Fatalf("the person's yes was not remembered at %s: %v", remembered, err)
	}

	// Saving over the skill changes the procedure the person said yes to, so the
	// yes goes with it and the next person has to be asked again.
	if err := built.store.Save(ctx, contract.SkillSavedByModel, "read-the-story", theSkillAModelWrote()); err != nil {
		t.Fatalf("saving the skill again failed: %v", err)
	}
	if _, err := os.Stat(remembered); !os.IsNotExist(err) {
		t.Errorf("the person's yes is still remembered at %s after the skill was saved over, and it stands for a procedure that is gone", remembered)
	}
}

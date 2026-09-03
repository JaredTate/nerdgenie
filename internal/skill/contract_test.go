package skill_test

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheRealStoreKeepsTheSkillContract(t *testing.T) {
	built := newHarness(t)
	if err := testkit.CheckSkill(context.Background(), built.store); err != nil {
		t.Fatalf("the real skill store broke the contract the fake keeps: %v", err)
	}
}

func TestTheRealStoreIsAContractSkill(t *testing.T) {
	built := newHarness(t)
	var skills contract.Skill = built.store
	if _, err := skills.List(context.Background()); err != nil {
		t.Fatalf("listing through the contract failed: %v", err)
	}
	var approver skill.StandingApprover
	if approver != nil {
		t.Error("the empty standing approver is not empty")
	}
}

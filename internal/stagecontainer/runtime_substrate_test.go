package stagecontainer

import "testing"

func TestStageImageRefIsDerivedOnlyFromStage(t *testing.T) {
	if got := StageImageRef("plan"); got != "autodev-stage-plan:current" {
		t.Fatalf("unexpected stage image ref: %s", got)
	}
	if got := BaseImageRef(); got != "autodev-stage-base:current" {
		t.Fatalf("unexpected base image ref: %s", got)
	}
}


package config

import (
	"path/filepath"
	"testing"
)

func TestLoadStageSpecs(t *testing.T) {
	specs, err := LoadStageSpecs(filepath.Join("..", "..", "TEST", "stage-specs"))
	if err != nil {
		t.Fatalf("LoadStageSpecs() error = %v", err)
	}
	if len(specs) != 19 {
		t.Fatalf("expected 19 stage specs, got %d", len(specs))
	}

	foundPromotion := false
	for _, spec := range specs {
		if spec.Name == "promote_prod" {
			foundPromotion = true
			if !spec.ApprovalRequired {
				t.Fatalf("promote_prod stage should require approval")
			}
		}
	}
	if !foundPromotion {
		t.Fatal("promote_prod stage not loaded")
	}
}

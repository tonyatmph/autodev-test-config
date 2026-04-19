package runner

import (
	"context"
	"testing"
)

func TestProjectCurrentState(t *testing.T) {
	ledger := &MockLedger{State: make(map[string]SHA)}
	
	pA := &ProviderMock{Name: "A"}
	pB := &ProviderMock{Name: "B", Deps: []string{"A"}}
	
	catalog := &MockCatalog{
		Providers: map[string]Provider{
			"A": pA,
			"B": pB,
		},
	}

	resolver := &Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
	}

	// 1. Resolve to populate ledger
	goal := Goal{Contract: Contract{Name: "B", Version: "1.0"}, InputSHA: "SHA-0"}
	resolver.Resolve(context.Background(), goal)

	// 2. Project
	projection, err := resolver.ProjectCurrentState(context.Background(), goal)
	if err != nil {
		t.Fatalf("Projection failed: %v", err)
	}

	if _, ok := projection.Nodes["A"]; !ok {
		t.Errorf("Expected node A in projection")
	}
	if _, ok := projection.Nodes["B"]; !ok {
		t.Errorf("Expected node B in projection")
	}
	
	t.Logf("Projection Nodes: %+v", projection.Nodes)
}

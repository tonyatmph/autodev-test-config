package runner

import (
	"context"
	"testing"
)

// ProviderMock wraps runner.Provider
type ProviderMock struct {
	Name      string
	Deps      []string
	ExecCount int
}

func (m *ProviderMock) Execute(ctx context.Context, inputs []SHA, stream TelemetryStream) (SHA, float64, map[string]float64, error) {
	m.ExecCount++
	// Fitness is 1.0 (pass), stats empty
	return SHA("SHA-FROM-" + m.Name), 1.0, nil, nil
}

func (p *ProviderMock) Dependencies(ctx context.Context, input SHA) ([]Goal, error) {
	var deps []Goal
	for _, d := range p.Deps {
		deps = append(deps, Goal{Contract: Contract{Name: d, Version: "1.0"}, InputSHA: input})
	}
	return deps, nil
}

// Concrete implementation to allow embedding
type BaseProvider struct {
	deps []string
}

func (p *BaseProvider) Dependencies(ctx context.Context, input SHA) ([]Goal, error) {
	var deps []Goal
	for _, depName := range p.deps {
		deps = append(deps, Goal{
			Contract: Contract{Name: depName, Version: "1.0"},
			InputSHA: input,
		})
	}
	return deps, nil
}

func (p *BaseProvider) Execute(ctx context.Context, inputs []SHA) (SHA, float64, map[string]float64, error) {
	return "SHA-RESULT", 1.0, nil, nil
}

func TestResolver(t *testing.T) {
	ledger := &MockLedger{State: make(map[string]SHA)}
	
	pA := &ProviderMock{Name: "A", Deps: nil}
	pB := &ProviderMock{Name: "B", Deps: []string{"A"}}
	pC := &ProviderMock{Name: "C", Deps: []string{"B"}}

	catalog := &MockCatalog{
		Providers: map[string]Provider{
			"A": pA,
			"B": pB,
			"C": pC,
		},
	}

	resolver := &Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
		FitnessThreshold: 0.9,
	}

	ctx := context.Background()
	_, err := resolver.Resolve(ctx, Goal{Contract: Contract{Name: "C", Version: "1.0"}, InputSHA: "SHA-0"})
	if err != nil {
		t.Fatalf("Resolution failed: %v", err)
	}

	if pA.ExecCount != 1 || pB.ExecCount != 1 || pC.ExecCount != 1 {
		t.Errorf("Expected execution counts (1,1,1), got (%d,%d,%d)", pA.ExecCount, pB.ExecCount, pC.ExecCount)
	}
}

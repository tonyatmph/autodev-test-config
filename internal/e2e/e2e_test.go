package e2e

import (
	"context"
	"fmt"
	"testing"
	"g7.mph.tech/mph-tech/autodev/internal/runner"
)

// Mocks
type MockLedger struct { State map[string]runner.SHA }
func (m *MockLedger) Lookup(ctx context.Context, c runner.Contract, i runner.SHA) (runner.SHA, bool) {
	key := fmt.Sprintf("%s:%s", c.Name, i)
	sha, ok := m.State[key]
	return sha, ok
}
func (m *MockLedger) Commit(ctx context.Context, c runner.Contract, i, o runner.SHA, stats map[string]float64) error {
	key := fmt.Sprintf("%s:%s", c.Name, i)
	m.State[key] = o
	return nil
}
func (m *MockLedger) Load(ctx context.Context, sha runner.SHA) ([]byte, error) { return []byte("mock"), nil }
func (m *MockLedger) Store(ctx context.Context, data []byte) (runner.SHA, error) { return "SHA-STORED", nil }

type MockCatalog struct { Providers map[string]runner.Provider }
func (m *MockCatalog) Find(ctx context.Context, c runner.Contract) (runner.Provider, error) {
	p, ok := m.Providers[c.Name]
	if !ok { return nil, fmt.Errorf("no provider") }
	return p, nil
}
func (m *MockCatalog) Build(ctx context.Context, c runner.Contract) error { return nil }

type ProviderMock struct {
	Name      string
	Deps      []string
	ExecCount int
	Fitness   float64
}
func (p *ProviderMock) Dependencies(ctx context.Context, input runner.SHA) ([]runner.Goal, error) {
	var deps []runner.Goal
	for _, d := range p.Deps {
		deps = append(deps, runner.Goal{Contract: runner.Contract{Name: d, Version: "1.0"}, InputSHA: input})
	}
	return deps, nil
}
func (p *ProviderMock) Execute(ctx context.Context, inputs []runner.SHA, stream runner.TelemetryStream) (runner.SHA, float64, map[string]float64, error) {
	p.ExecCount++
	fitness := 1.0
	if p.Fitness != 0 { fitness = p.Fitness }
	return runner.SHA("SHA-FROM-" + p.Name), fitness, nil, nil
}

func TestFullPipelineResolution(t *testing.T) {
	ledger := &MockLedger{State: make(map[string]runner.SHA)}
	pImpl := &ProviderMock{Name: "implement", Deps: nil}
	pSec  := &ProviderMock{Name: "security", Deps: []string{"implement"}}
	pComp := &ProviderMock{Name: "complexity", Deps: []string{"security"}}
	pPkg  := &ProviderMock{Name: "package", Deps: []string{"complexity"}}

	catalog := &MockCatalog{
		Providers: map[string]runner.Provider{
			"implement":  pImpl,
			"security":   pSec,
			"complexity": pComp,
			"package":    pPkg,
		},
	}

	resolver := &runner.Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
		FitnessThreshold: 0.9,
	}

	_, err := resolver.Resolve(context.Background(), runner.Goal{
		Contract: runner.Contract{Name: "package", Version: "1.0"}, 
		InputSHA: "SHA-INITIAL",
	})
	
	if err != nil {
		t.Fatalf("Full pipeline resolution failed: %v", err)
	}
}

func TestChaosMonkeyRecovery(t *testing.T) {
	ledger := &MockLedger{State: make(map[string]runner.SHA)}
	
	pSec := &ProviderMock{Name: "security-probe", Deps: nil, Fitness: 0.5}

	catalog := &MockCatalog{
		Providers: map[string]runner.Provider{
			"security-probe": pSec,
		},
	}

	resolver := &runner.Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
		FitnessThreshold: 0.9,
	}

	_, err := resolver.Resolve(context.Background(), runner.Goal{
		Contract: runner.Contract{Name: "security-probe", Version: "1.0"}, 
		InputSHA: "SHA-INITIAL",
	})
	
	if err == nil {
		t.Error("Expected fitness failure, but resolution succeeded")
	}
}

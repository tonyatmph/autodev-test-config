package runner

import (
	"context"
	"fmt"
)

// MockLedger implements Ledger.
type MockLedger struct {
	State map[string]SHA
}

func (m *MockLedger) Lookup(ctx context.Context, contract Contract, input SHA) (SHA, bool) {
	key := fmt.Sprintf("%s:%s", contract.Name, input)
	sha, ok := m.State[key]
	return sha, ok
}
func (m *MockLedger) Commit(ctx context.Context, contract Contract, input, output SHA, stats map[string]float64) error {
	key := fmt.Sprintf("%s:%s", contract.Name, input)
	m.State[key] = output
	return nil
}
func (m *MockLedger) Load(ctx context.Context, sha SHA) ([]byte, error) { return []byte("mock"), nil }
func (m *MockLedger) Store(ctx context.Context, data []byte) (SHA, error) { return "SHA-STORED", nil }

// MockCatalog implements Catalog.
type MockCatalog struct {
	Providers map[string]Provider
}

func (m *MockCatalog) Find(ctx context.Context, contract Contract) (Provider, error) {
	p, ok := m.Providers[contract.Name]
	if !ok {
		return nil, fmt.Errorf("no provider")
	}
	return p, nil
}
func (m *MockCatalog) Build(ctx context.Context, contract Contract) error {
	return nil
}

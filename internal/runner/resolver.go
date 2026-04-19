package runner

import (
	"context"
	"fmt"
)

// SHA represents a cryptographic hash of a state in the ledger.
type SHA string

// Contract defines the schema/interface a state transition must satisfy.
type Contract struct {
	Name    string
	Version string
}

// Goal represents the target state we want to reach.
type Goal struct {
	Contract Contract
	InputSHA SHA
}

// Provider represents a "Factory" or "Cell" capable of satisfying a Contract.
type Provider interface {
	Dependencies(ctx context.Context, input SHA) ([]Goal, error)
	// Execute performs the state transition and returns the new SHA, fitness, and optional stats.
	// The TelemetryStream allows the provider to pipe live execution events (logs, metrics) back.
	Execute(ctx context.Context, inputs []SHA, stream TelemetryStream) (SHA, float64, map[string]float64, error)
}

// Ledger is the shared memory.
type Ledger interface {
	Lookup(ctx context.Context, contract Contract, input SHA) (SHA, bool)
	Commit(ctx context.Context, contract Contract, input, output SHA, stats map[string]float64) error
	
	// Artifact management: Store/Load content-addressable artifacts
	Store(ctx context.Context, data []byte) (SHA, error)
	Load(ctx context.Context, sha SHA) ([]byte, error)
}

// Catalog is the registry of all available capabilities.
type Catalog interface {
	Find(ctx context.Context, contract Contract) (Provider, error)
	Build(ctx context.Context, contract Contract) error
}

// Resolver is the JIT Graph Compiler.
type Resolver struct {
	Ledger           Ledger
	Catalog          Catalog
	FitnessThreshold float64
}

// NewResolver creates a new Resolver.
func NewResolver(ledger Ledger, catalog Catalog, fitnessThreshold float64) *Resolver {
	return &Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
		FitnessThreshold: fitnessThreshold,
	}
}

// Resolve recursively evaluates the dependency graph to satisfy a Goal.
func (r *Resolver) Resolve(ctx context.Context, goal Goal) (SHA, error) {
	if cachedSHA, found := r.Ledger.Lookup(ctx, goal.Contract, goal.InputSHA); found {
		return cachedSHA, nil
	}

	provider, err := r.Catalog.Find(ctx, goal.Contract)
	if err != nil {
		if err := r.Catalog.Build(ctx, goal.Contract); err != nil {
			return "", fmt.Errorf("capability build failed for %s: %w", goal.Contract.Name, err)
		}
		provider, err = r.Catalog.Find(ctx, goal.Contract)
		if err != nil {
			return "", fmt.Errorf("capability still missing after build for %s: %w", goal.Contract.Name, err)
		}
	}

	deps, err := provider.Dependencies(ctx, goal.InputSHA)
	if err != nil {
		return "", fmt.Errorf("failed to compute dependencies for %s: %w", goal.Contract.Name, err)
	}

	resolvedInputs := make([]SHA, 0, len(deps)+1)
	resolvedInputs = append(resolvedInputs, goal.InputSHA)

	for _, depGoal := range deps {
		depSHA, err := r.Resolve(ctx, depGoal)
		if err != nil {
			return "", fmt.Errorf("failed to resolve dependency %s: %w", depGoal.Contract.Name, err)
		}
		resolvedInputs = append(resolvedInputs, depSHA)
	}

	var outputSHA SHA
	var lastErr error
	var lastStats map[string]float64
	success := false
	
	// Initialize telemetry stream
	stream := make(TelemetryStream, 100)
	defer close(stream)

	// Try execution up to 3 times (Backtracking/Recovery loop)
	for i := 0; i < 3; i++ {
		var fitness float64
		outputSHA, fitness, lastStats, err = provider.Execute(ctx, resolvedInputs, stream)
		if err == nil && fitness >= r.FitnessThreshold {
			success = true
			break
		}
		lastErr = err
	}

	if !success {
		return "", fmt.Errorf("provider failed after retries: %v", lastErr)
	}

	if err := r.Ledger.Commit(ctx, goal.Contract, goal.InputSHA, outputSHA, lastStats); err != nil {
		return "", fmt.Errorf("failed to commit transition to ledger: %w", err)
	}

	return outputSHA, nil
}

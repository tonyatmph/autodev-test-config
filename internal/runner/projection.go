package runner

import (
	"context"
)

// GraphProjection represents the live state of the pipeline as a graph.
type GraphProjection struct {
	Nodes map[string]NodeProjection `json:"nodes"`
	Edges []EdgeProjection          `json:"edges"`
}

type NodeProjection struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"` // pending, running, succeeded, failed
	Fitness     float64 `json:"fitness"`
	LastSHA     SHA     `json:"last_sha"`
}

type EdgeProjection struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ProjectCurrentState resolves the live DAG and state from the Ledger.
func (r *Resolver) ProjectCurrentState(ctx context.Context, goal Goal) (*GraphProjection, error) {
	projection := &GraphProjection{
		Nodes: make(map[string]NodeProjection),
		Edges: []EdgeProjection{},
	}

	// Recursive traversal to build the graph
	var traverse func(g Goal) error
	traverse = func(g Goal) error {
		// 1. Resolve Provider
		provider, err := r.Catalog.Find(ctx, g.Contract)
		if err != nil {
			return nil // Skip unknown providers for now
		}

		// 2. Identify Status from Ledger
		status := "pending"
		sha, found := r.Ledger.Lookup(ctx, g.Contract, g.InputSHA)
		if found {
			status = "succeeded"
		}

		projection.Nodes[g.Contract.Name] = NodeProjection{
			Name:    g.Contract.Name,
			Status:  status,
			LastSHA: sha,
		}

		// 3. Recurse
		deps, _ := provider.Dependencies(ctx, g.InputSHA)
		for _, dep := range deps {
			projection.Edges = append(projection.Edges, EdgeProjection{From: dep.Contract.Name, To: g.Contract.Name})
			if err := traverse(dep); err != nil {
				return err
			}
		}
		return nil
	}

	err := traverse(goal)
	return projection, err
}

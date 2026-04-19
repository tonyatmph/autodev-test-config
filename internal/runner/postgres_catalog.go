package runner

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

// PostgresCatalog implements the Catalog interface using Postgres.
type PostgresCatalog struct {
	pool         *pgxpool.Pool
	dockerRunner stagecontainer.DockerRunner
}

func NewPostgresCatalog(pool *pgxpool.Pool, dockerRunner stagecontainer.DockerRunner) *PostgresCatalog {
	return &PostgresCatalog{
		pool:         pool,
		dockerRunner: dockerRunner,
	}
}

// Find resolves a Contract to a Provider implementation by querying Postgres.
func (c *PostgresCatalog) Find(ctx context.Context, contract Contract) (Provider, error) {
	// 1. Query the capabilities table
	var imageRef string
	var entrypoint []string
	
	// Example query: SELECT image_ref, entrypoint FROM capabilities WHERE name = $1
	query := "SELECT image_ref, entrypoint FROM capabilities WHERE name = $1"
	err := c.pool.QueryRow(ctx, query, contract.Name).Scan(&imageRef, &entrypoint)
	if err != nil {
		return nil, fmt.Errorf("capability %s not found in catalog: %w", contract.Name, err)
	}

	// 2. Return a ContainerProvider for the resolved capability
	return NewContainerProvider(
		model.StageSpec{
			Name:       contract.Name,
			Entrypoint: entrypoint,
			ToolingRepo: model.ToolingRepo{URL: imageRef},
			Version:    "latest", // Or derived from DB
		},
		c.dockerRunner,
		stagecontainer.Config{},
	), nil
}

func (c *PostgresCatalog) Build(ctx context.Context, contract Contract) error {
	// Placeholder: In a production system, this would trigger an 
	// automated image build stage and update the capabilities table.
	return nil
}

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

// ContainerProvider implements the Provider interface using a Docker container.
type ContainerProvider struct {
	spec       model.StageSpec
	dockerExec stagecontainer.DockerRunner
	dockerCfg  stagecontainer.Config
}

func NewContainerProvider(spec model.StageSpec, dockerExec stagecontainer.DockerRunner, dockerCfg stagecontainer.Config) *ContainerProvider {
	return &ContainerProvider{
		spec:       spec,
		dockerExec: dockerExec,
		dockerCfg:  dockerCfg,
	}
}

type StageResult struct {
	Status  string         `json:"status"`
	NewSHA  string         `json:"new_sha"`
	Fitness float64        `json:"fitness"`
}

// Dependencies returns the sub-goals required before this provider can execute.
func (p *ContainerProvider) Dependencies(ctx context.Context, input SHA) ([]Goal, error) {
	var deps []Goal
	for _, depName := range p.spec.Dependencies {
		deps = append(deps, Goal{
			Contract: Contract{Name: depName, Version: p.spec.Version},
			InputSHA: input,
		})
	}
	return deps, nil
}

// Execute performs the state transition by triggering the container.
func (p *ContainerProvider) Execute(ctx context.Context, inputs []SHA, stream TelemetryStream) (SHA, float64, map[string]float64, error) {
	// 1. Trigger the container (Cell).
	runID := "e2e-run-placeholder"
	image := stagecontainer.RuntimeImage{
		Ref:    p.spec.ToolingRepo.URL, // e.g., "alpine"
		Digest: p.spec.Version,         // e.g., "latest"
	}
	// Combine them into a full reference for Docker
	fullImage := fmt.Sprintf("%s:%s", image.Ref, image.Digest)
	
	err := p.dockerExec.Run(ctx, p.dockerCfg, p.spec, runID, stagecontainer.RuntimeImage{
		Ref:    fullImage,
		Digest: image.Digest,
	})
	if err != nil {
		return "", 0, nil, fmt.Errorf("container execution failed: %w", err)
	}

	// 2. Read the result.json from the workspace.
	resultPath := filepath.Join("/workspace", "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		return "", 0, nil, fmt.Errorf("failed to read result.json: %w", err)
	}

	var res StageResult
	if err := json.Unmarshal(data, &res); err != nil {
		return "", 0, nil, fmt.Errorf("failed to parse result.json: %w", err)
	}

	return SHA(res.NewSHA), res.Fitness, nil, nil
}

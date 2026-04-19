package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

// BinaryProvider implements the Provider interface by executing a binary as a separate process.
type BinaryProvider struct {
	spec model.StageSpec
}

func NewBinaryProvider(spec model.StageSpec) *BinaryProvider {
	return &BinaryProvider{
		spec: spec,
	}
}

// Dependencies returns the sub-goals required before this provider can execute.
func (p *BinaryProvider) Dependencies(ctx context.Context, input SHA) ([]Goal, error) {
	var deps []Goal
	for _, depName := range p.spec.Dependencies {
		deps = append(deps, Goal{
			Contract: Contract{Name: depName, Version: p.spec.Version},
			InputSHA: input,
		})
	}
	return deps, nil
}

// Execute performs the state transition by triggering the binary process.
func (p *BinaryProvider) Execute(ctx context.Context, inputs []SHA) (SHA, float64, map[string]float64, error) {
	if len(p.spec.Entrypoint) == 0 {
		return "", 0, nil, fmt.Errorf("no entrypoint defined for binary provider")
	}

	// Trigger the binary as a separate OS process
	cmd := exec.CommandContext(ctx, p.spec.Entrypoint[0], p.spec.Entrypoint[1:]...)
	
	// Ensure isolation by limiting the environment
	cmd.Env = []string{
		"AUTODEV_STAGE_CONTEXT=/workspace/context.json",
		"AUTODEV_STAGE_RESULT=/workspace/result.json",
		"AUTODEV_STAGE_REPORT=/workspace/report.json",
	}

	if err := cmd.Run(); err != nil {
		return "", 0, nil, fmt.Errorf("binary execution failed: %w", err)
	}

	// Read the result.json from the workspace
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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func LoadStageSpecs(dir string) ([]model.StageSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read specs dir: %w", err)
	}

	specs := make([]model.StageSpec, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read spec %s: %w", path, err)
		}
		if err := contracts.Validate(contracts.StageSpecSchema, path, data); err != nil {
			return nil, err
		}

		var spec model.StageSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("decode spec %s: %w", path, err)
		}
		if spec.Name == "" {
			return nil, fmt.Errorf("spec %s missing stage name", path)
		}
		if strings.TrimSpace(spec.Operation) == "" {
			return nil, fmt.Errorf("spec %s missing stage operation", path)
		}
		if strings.TrimSpace(spec.Operation) != "orchestrate" {
			return nil, fmt.Errorf("spec %s must use operation %q, got %q", path, "orchestrate", strings.TrimSpace(spec.Operation))
		}
		plan, err := spec.OrchestrationPlan()
		if err != nil {
			return nil, fmt.Errorf("decode operation plan %s: %w", path, err)
		}
		if len(plan.Steps) == 0 {
			return nil, fmt.Errorf("spec %s missing orchestration steps", path)
		}
		for i, step := range plan.Steps {
			if len(step.Command) == 0 {
				return nil, fmt.Errorf("spec %s step %d missing command", path, i+1)
			}
			for _, item := range step.Command {
				if strings.TrimSpace(item) == "" {
					return nil, fmt.Errorf("spec %s step %d has empty command element", path, i+1)
				}
			}
		}
		if spec.SummaryText() == "" {
			return nil, fmt.Errorf("spec %s missing runtime summary", path)
		}
		if spec.QueueMode() == "" {
			return nil, fmt.Errorf("spec %s missing runtime.queue_mode", path)
		}
		if spec.QueueMode() != model.StageQueueModeAuto && spec.QueueMode() != model.StageQueueModeTriggered {
			return nil, fmt.Errorf("spec %s has invalid runtime.queue_mode %q", path, spec.QueueMode())
		}
		container := spec.ContainerConfig()
		if !container.RunAs.Valid() {
			return nil, fmt.Errorf("spec %s missing valid container.run_as", path)
		}
		if !container.WriteAs.Valid() {
			return nil, fmt.Errorf("spec %s missing valid container.write_as", path)
		}
		if container.Permissions.Network == "" {
			return nil, fmt.Errorf("spec %s missing container.permissions.network", path)
		}
		if container.Permissions.RuntimeUser.Mode == "" {
			return nil, fmt.Errorf("spec %s missing container.permissions.runtime_user.mode", path)
		}
		if strings.TrimSpace(spec.Runtime.Stats.Model) == "" {
			return nil, fmt.Errorf("spec %s missing runtime.stats.model", path)
		}
		if spec.Runtime.Stats.ToolCalls <= 0 {
			return nil, fmt.Errorf("spec %s missing runtime.stats.tool_calls", path)
		}
		if spec.Runtime.Stats.ToolingUSD <= 0 {
			return nil, fmt.Errorf("spec %s missing runtime.stats.tooling_usd", path)
		}
		if spec.Runtime.Stats.ExpectedArtifacts <= 0 {
			return nil, fmt.Errorf("spec %s missing runtime.stats.expected_artifacts", path)
		}
		if spec.SuccessCriteriaContract().RequireSummary && spec.SummaryText() == "" {
			return nil, fmt.Errorf("spec %s requires a summary but provides none", path)
		}
		spec.Normalize()
		specs = append(specs, spec)
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})

	return specs, nil
}

func SpecMap(specs []model.StageSpec) map[string]model.StageSpec {
	out := make(map[string]model.StageSpec, len(specs))
	for _, spec := range specs {
		out[spec.Name] = spec
	}
	return out
}

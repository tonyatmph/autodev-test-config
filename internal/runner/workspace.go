package runner

import (
	"fmt"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
)

func (e *StageExecutor) loadWorkspaceJSON(runID, stage, name string, dest any) error {
	path := filepath.Join(e.dataDir, "workspaces", runID, stage, name)
	if err := contracts.ReadFile(path, workspaceSchema(name), dest); err != nil {
		return fmt.Errorf("load workspace %s/%s/%s: %w", runID, stage, name, err)
	}
	return nil
}

func workspaceSchema(name string) string {
	switch name {
	case "context.json":
		return contracts.StageContextSchema
	case "result.json":
		return contracts.StageResultSchema
	case "report.json":
		return contracts.StageReportSchema
	case "state.json":
		return contracts.StageStateSchema
	default:
		return ""
	}
}

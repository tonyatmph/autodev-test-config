package workorder

import (
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

// Writer writes work-order artifacts into the journal repository.
type Writer struct {
	repoPath string
}

// NewWriter constructs a journal writer bound to the given repo path.
func NewWriter(repoPath string) *Writer {
	return &Writer{repoPath: repoPath}
}

// WritePipelineArtifact writes a single pipeline artifact into the journal.
func (w *Writer) WritePipelineArtifact(run model.RunRequest, key string, value any) error {
	if w == nil || w.repoPath == "" || value == nil {
		return nil
	}
	path := filepath.Join(w.repoPath, "work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID), "pipeline", key+".json")
	return contracts.WriteFile(path, schemaForPipelineArtifact(key), value)
}

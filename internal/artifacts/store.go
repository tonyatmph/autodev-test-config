package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) PutJSON(runID, stage, name string, value any, retention string) (model.ArtifactRef, error) {
	dir := filepath.Join(s.root, runID, stage)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.ArtifactRef{}, fmt.Errorf("create artifact dir: %w", err)
	}

	payload, err := contracts.Marshal(artifactSchema(name), filepath.Join(dir, name+".json"), value)
	if err != nil {
		return model.ArtifactRef{}, fmt.Errorf("encode artifact %s: %w", name, err)
	}

	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return model.ArtifactRef{}, fmt.Errorf("write artifact %s: %w", name, err)
	}

	digest := sha256.Sum256(payload)
	return model.ArtifactRef{
		Name:            name,
		URI:             path,
		Digest:          "sha256:" + hex.EncodeToString(digest[:]),
		ProducerStage:   stage,
		RetentionPolicy: retention,
		Metadata: map[string]any{
			"size": len(payload),
		},
	}, nil
}

func (s *Store) ReadJSON(runID, stage, name string, dest any) error {
	path := filepath.Join(s.root, runID, stage, name+".json")
	if err := contracts.ReadFile(path, artifactSchema(name), dest); err != nil {
		return fmt.Errorf("load artifact %s/%s/%s: %w", runID, stage, name, err)
	}
	return nil
}

func artifactSchema(name string) string {
	switch name {
	case "result":
		return contracts.StageResultSchema
	case "evidence":
		return contracts.StageReportSchema
	default:
		return ""
	}
}

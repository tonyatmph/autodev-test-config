package configsource

import (
	"fmt"
	"os"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/config"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

const fixedConfigRel = ".autodev/config"

func Root() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("resolve fixed config root: %v", err))
	}
	return filepath.Join(home, fixedConfigRel)
}

func StageSpecsDir() string {
	return filepath.Join(Root(), "stage-specs")
}

func PipelineCatalogPath() string {
	return filepath.Join(Root(), "pipelines", "catalog.json")
}

func LoadStageSpecs() ([]model.StageSpec, error) {
	return config.LoadStageSpecs(StageSpecsDir())
}

func LoadPipelineCatalog() (map[string]any, error) {
	path := PipelineCatalogPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline catalog: %w", err)
	}
	var catalog map[string]any
	if err := contracts.Unmarshal(data, contracts.PipelineCatalogSchema, path, &catalog); err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, fmt.Errorf("pipeline catalog is empty")
	}
	return catalog, nil
}

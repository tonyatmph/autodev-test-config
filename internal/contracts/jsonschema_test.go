package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCheckedInConfig(t *testing.T) {
	path := filepath.Join("..", "..", "autodev.config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := Validate(ConfigSchema, path, data); err != nil {
		t.Fatalf("validate config: %v", err)
	}
}

func TestValidateCheckedInIssueFile(t *testing.T) {
	path := filepath.Join("..", "..", "hack", "local-sample-issue.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read issue file: %v", err)
	}
	if err := Validate(IssueFileSchema, path, data); err != nil {
		t.Fatalf("validate issue file: %v", err)
	}
}

func TestValidateCheckedInStageSpec(t *testing.T) {
	path := filepath.Join("..", "..", "TEST", "stage-specs", "plan.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage spec: %v", err)
	}
	if err := Validate(StageSpecSchema, path, data); err != nil {
		t.Fatalf("validate stage spec: %v", err)
	}
}

func TestValidateCheckedInPipelineCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "TEST", "pipelines", "catalog.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pipeline catalog: %v", err)
	}
	if err := Validate(PipelineCatalogSchema, path, data); err != nil {
		t.Fatalf("validate pipeline catalog: %v", err)
	}
}

func TestConfigSchemaRejectsLegacyStageExecutionMode(t *testing.T) {
	payload := []byte(`{
	  "paths": {
	    "root_dir": ".",
	    "data_dir": "/tmp/autodev-data",
	    "work_order_repo": "/tmp/work-orders"
	  },
	  "gitlab": {
	    "token_name": "gitlab-token"
	  },
	  "stores": {},
	  "secrets": {
	    "local_keychain_service": "autodev"
	  },
	  "execution": {
	    "stage_execution_mode": "container"
	  }
	}`)
	if err := Validate(ConfigSchema, "legacy-config.json", payload); err == nil {
		t.Fatal("expected legacy stage_execution_mode field to be rejected")
	}
}

func TestConfigSchemaRejectsLegacyStageRunnerImage(t *testing.T) {
	payload := []byte(`{
	  "paths": {
	    "root_dir": ".",
	    "data_dir": "/tmp/autodev-data",
	    "work_order_repo": "/tmp/work-orders"
	  },
	  "gitlab": {
	    "token_name": "gitlab-token"
	  },
	  "stores": {},
	  "secrets": {
	    "local_keychain_service": "autodev"
	  },
	  "execution": {
	    "stage_runner_image": "autodev-stage-runner:local"
	  }
	}`)
	if err := Validate(ConfigSchema, "legacy-config.json", payload); err == nil {
		t.Fatal("expected legacy stage_runner_image field to be rejected")
	}
}

func TestConfigSchemaRejectsLegacyImageCatalogPath(t *testing.T) {
	payload := []byte(`{
	  "paths": {
	    "root_dir": ".",
	    "data_dir": "/tmp/autodev-data",
	    "work_order_repo": "/tmp/work-orders"
	  },
	  "gitlab": {
	    "token_name": "gitlab-token"
	  },
	  "stores": {},
	  "secrets": {
	    "local_keychain_service": "autodev"
	  },
	  "execution": {
	    "image_catalog": "containers/image-catalog.json"
	  }
	}`)
	if err := Validate(ConfigSchema, "legacy-config.json", payload); err == nil {
		t.Fatal("expected legacy image_catalog field to be rejected")
	}
}

func TestConfigSchemaRejectsLegacySpecDirAndPipelineCatalogPaths(t *testing.T) {
	payload := []byte(`{
	  "paths": {
	    "root_dir": ".",
	    "data_dir": "/tmp/autodev-data",
	    "spec_dir": "stage-specs",
	    "work_order_repo": "/tmp/work-orders",
	    "pipeline_catalog": "pipelines/catalog.json"
	  },
	  "gitlab": {
	    "token_name": "gitlab-token"
	  },
	  "stores": {},
	  "secrets": {
	    "local_keychain_service": "autodev"
	  }
	}`)
	if err := Validate(ConfigSchema, "legacy-config.json", payload); err == nil {
		t.Fatal("expected legacy spec_dir and pipeline_catalog fields to be rejected")
	}
}

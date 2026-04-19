package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStageSpecsUseUniversalOrchestrateOperation(t *testing.T) {
	root := repoRoot(t)
	for _, source := range []string{"PROD", "TEST"} {
		specDir := filepath.Join(root, source, "stage-specs")
		entries, err := os.ReadDir(specDir)
		if err != nil {
			t.Fatalf("read %s stage-specs: %v", source, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			path := filepath.Join(specDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var payload struct {
				Name            string         `json:"name"`
				Operation       string         `json:"operation"`
				OperationConfig map[string]any `json:"operation_config"`
				Entrypoint      []string       `json:"entrypoint"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			if payload.Operation != "orchestrate" {
				t.Fatalf("%s: expected operation %q, got %q", path, "orchestrate", payload.Operation)
			}
			steps, ok := payload.OperationConfig["steps"].([]any)
			if !ok || len(steps) == 0 {
				t.Fatalf("%s: expected non-empty operation_config.steps", path)
			}
			for _, raw := range steps {
				step, ok := raw.(map[string]any)
				if !ok {
					t.Fatalf("%s: expected step object, got %T", path, raw)
				}
				if _, ok := step["action"]; ok {
					t.Fatalf("%s: action-based steps are forbidden", path)
				}
				command, ok := step["command"].([]any)
				if !ok || len(command) == 0 {
					t.Fatalf("%s: expected non-empty command step", path)
				}
				command0, _ := command[0].(string)
				if command0 == "python3" || command0 == "/usr/bin/python3" {
					t.Fatalf("%s: stage commands must not depend on Python in the universal runtime image", path)
				}
			}
			if len(payload.Entrypoint) != 1 || payload.Entrypoint[0] != "autodev-stage-runtime" {
				t.Fatalf("%s: expected autodev-stage-runtime entrypoint, got %v", path, payload.Entrypoint)
			}
		}
	}
}

func TestNonTestGoDoesNotDispatchOnNamedStages(t *testing.T) {
	root := repoRoot(t)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`switch\s+spec\.Name\b`),
		regexp.MustCompile(`switch\s+attempt\.Stage\b`),
		regexp.MustCompile(`spec\.Name\s*==\s*"[^"]+"`),
		regexp.MustCompile(`attempt\.Stage\s*==\s*"[^"]+"`),
		regexp.MustCompile(`strings\.HasPrefix\(\s*spec\.Name\s*,\s*"[^"]+"`),
		regexp.MustCompile(`strings\.HasPrefix\(\s*attempt\.Stage\s*,\s*"[^"]+"`),
		regexp.MustCompile(`universal_stage\.py`),
		regexp.MustCompile(`"action"\s*:`),
	}

	for _, path := range nonTestGoFiles(t, root) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for _, re := range forbidden {
			if re.FindStringIndex(body) != nil {
				t.Fatalf("%s: contains forbidden stage-specific dispatch pattern %q", path, re.String())
			}
		}
	}
}

func TestRuntimeAndJournalJSONUseContractHelpers(t *testing.T) {
	root := repoRoot(t)
	targets := []string{
		"tooling/runtime/single_plane.go",
		"internal/runner/workspace.go",
		"internal/artifacts/store.go",
		"internal/store/store.go",
		"internal/stagecontainer/runtime_substrate.go",
		"internal/workorder/pipeline_plan.go",
		"internal/workorder/journal_writer.go",
		"internal/workorder/journal_reader.go",
	}
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`json\.Marshal\(`),
		regexp.MustCompile(`json\.MarshalIndent\(`),
		regexp.MustCompile(`json\.Unmarshal\(`),
	}

	for _, rel := range targets {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for _, re := range forbidden {
			if re.FindStringIndex(body) != nil {
				t.Fatalf("%s: contains forbidden raw json helper %q", path, re.String())
			}
		}
	}
}

func TestTestHarnessDoesNotUseFakeDockerShim(t *testing.T) {
	root := repoRoot(t)
	targets := []string{
		"internal/runner/testmain_test.go",
		"cmd/stage-runner/local_smoke_test.go",
	}
	forbidden := []string{
		"autodev-fake-docker",
		"installFakeDocker(",
		"exec \"$ENTRYPOINT\" \"$@\"",
	}

	for _, rel := range targets {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for _, marker := range forbidden {
			if strings.Contains(body, marker) {
				t.Fatalf("%s: contains forbidden fake-docker harness marker %q", path, marker)
			}
		}
	}
}

func TestRuntimeTrustDoesNotUseLegacyCatalogOrSplitBrainProvenance(t *testing.T) {
	root := repoRoot(t)
	targets := []string{
		"cmd/stage-runner/main.go",
		"cmd/control-plane/main.go",
		"docker/runner/Dockerfile",
		"docker/control-plane/Dockerfile",
		"internal/configsource/configsource.go",
		"internal/stagecontainer/docker.go",
		"internal/stagecontainer/runtime_substrate.go",
		"tools/build_stage_images.py",
	}
	forbidden := []string{
		"/tmp/autodev-config",
		"image-catalog.json",
		"execution.image_catalog",
		"containercatalog",
		"source_image",
		"inherits_from",
		"base_digest",
		"base_reference",
	}

	for _, rel := range targets {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for _, marker := range forbidden {
			if strings.Contains(body, marker) {
				t.Fatalf("%s: contains forbidden legacy trust marker %q", path, marker)
			}
		}
	}
}

func TestRuntimeArtifactsDoNotHardcodeAConfigSource(t *testing.T) {
	root := repoRoot(t)
	targets := []string{
		"docker/runner/Dockerfile",
		"docker/control-plane/Dockerfile",
		"tools/build_stage_images.py",
	}
	forbidden := []string{
		"COPY TEST/stage-specs",
		"COPY TEST/pipelines",
		"COPY PROD/stage-specs",
		"COPY PROD/pipelines",
		`CONFIG_SOURCE = pathlib.Path("TEST")`,
		`CONFIG_SOURCE = pathlib.Path("PROD")`,
	}

	for _, rel := range targets {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for _, marker := range forbidden {
			if strings.Contains(body, marker) {
				t.Fatalf("%s: contains forbidden hardcoded config source marker %q", path, marker)
			}
		}
	}
}

func TestDeprecatedInvariantMarkerAbsentFromSourceTree(t *testing.T) {
	root := repoRoot(t)
	marker := "DEPRECIATED" + " INVARIANT"
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".autodev-home" || strings.HasPrefix(rel, ".autodev-home/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(rel, marker) {
			hits = append(hits, "path:"+rel)
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), marker) {
			hits = append(hits, "content:"+rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk source tree: %v", err)
	}
	if len(hits) > 0 {
		t.Fatalf("deprecated invariant marker present in source tree:\n%s", strings.Join(hits, "\n"))
	}
}

func nonTestGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"internal", "cmd"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			out = append(out, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

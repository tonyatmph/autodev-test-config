package workorder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

// Reader exposes helpers to load work-order journal summaries and reports.
type Reader struct {
	repoPath string
}

// NewReader constructs a journal reader bound to the work-order repository.
func NewReader(repoPath string) *Reader {
	return &Reader{repoPath: repoPath}
}

// ReadSummary loads the summary.json for the given run and attempt.
func (r *Reader) ReadSummary(run model.RunRequest, attempt model.StageAttempt) (map[string]any, error) {
	path := r.summaryPath(run, attempt)
	return r.readJSON(path, "")
}

// ReadReport loads the report.json for the given run and attempt.
func (r *Reader) ReadReport(run model.RunRequest, attempt model.StageAttempt) (map[string]any, error) {
	path := r.reportPath(run, attempt)
	return r.readJSON(path, "")
}

// ReadPipelineArtifact loads a pipeline artifact such as intent, builds, or execution plans.
func (r *Reader) ReadPipelineArtifact(run model.RunRequest, name string) (map[string]any, error) {
	path := r.pipelineArtifactPath(run, name)
	return r.readJSON(path, schemaForPipelineArtifact(name))
}

func (r *Reader) LatestAttempt(run model.RunRequest, stage string) (*model.StageAttempt, error) {
	stageDir := filepath.Join(r.repoPath, r.runDir(run), "stages", stage)
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return nil, fmt.Errorf("read stage dir %s: %w", stageDir, err)
	}
	var attempts []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "attempt-") {
			continue
		}
		num, err := strconv.Atoi(strings.TrimPrefix(name, "attempt-"))
		if err != nil {
			continue
		}
		attempts = append(attempts, num)
	}
	if len(attempts) == 0 {
		return nil, fmt.Errorf("no attempts found for stage %s", stage)
	}
	sort.Ints(attempts)
	return &model.StageAttempt{Stage: stage, Attempt: attempts[len(attempts)-1]}, nil
}

func (r *Reader) readJSON(path, schema string) (map[string]any, error) {
	_, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read journal file %s: %w", path, err)
	}
	var payload map[string]any
	if err := contracts.ReadFile(path, schema, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (r *Reader) summaryPath(run model.RunRequest, attempt model.StageAttempt) string {
	return filepath.Join(r.stageDir(run, attempt), "summary.json")
}

func (r *Reader) reportPath(run model.RunRequest, attempt model.StageAttempt) string {
	return filepath.Join(r.stageDir(run, attempt), "report.json")
}

func (r *Reader) stageDir(run model.RunRequest, attempt model.StageAttempt) string {
	return filepath.Join(r.repoPath, r.runDir(run), "stages", attempt.Stage, fmt.Sprintf("attempt-%02d", attempt.Attempt))
}

func (r *Reader) runDir(run model.RunRequest) string {
	return filepath.Join("work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID))
}

func (r *Reader) pipelineArtifactPath(run model.RunRequest, name string) string {
	return filepath.Join(r.repoPath, r.runDir(run), "pipeline", fmt.Sprintf("%s.json", name))
}

func canonicalWorkOrderID(run model.RunRequest) string {
	workOrder := run.CanonicalWorkOrder()
	if id := strings.TrimSpace(workOrder.ID); id != "" {
		return sanitizePathSegment(id)
	}
	return sanitizePathSegment(run.ID)
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "..", "-")
	return replacer.Replace(value)
}

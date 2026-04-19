package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
)

const (
	reportSchemaVersion = "autodev-stage-report-v1"
	stateSchemaVersion  = "autodev-stage-state-v2"
	maxCapture          = 1 << 20
)

type Contract struct {
	Stage struct {
		Name          string        `json:"name"`
		OperationPlan OperationPlan `json:"operation_plan"`
		Runtime       Runtime       `json:"runtime"`
	} `json:"stage"`
	Run       Ref            `json:"run"`
	Attempt   Attempt        `json:"attempt"`
	Issue     Issue          `json:"issue"`
	WorkOrder map[string]any `json:"work_order"`
}

type OperationPlan struct {
	Steps []Step `json:"steps"`
}

type Step struct {
	Command []string `json:"command"`
	Name    string   `json:"name,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
}

type Runtime struct {
	Summary     string `json:"summary,omitempty"`
	Transitions struct {
		OnSuccess []string `json:"on_success"`
		OnFailure []string `json:"on_failure"`
	} `json:"transitions"`
	SuccessCriteria struct {
		RequireSummary  bool     `json:"require_summary"`
		RequiredOutputs []string `json:"required_outputs"`
		RequiredMeta    []string `json:"required_report_metadata"`
	} `json:"success_criteria"`
}

type Ref struct {
	ID string `json:"id"`
}

type Attempt struct {
	ID      string `json:"id"`
	Attempt int    `json:"attempt"`
}

type Issue struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type Result struct {
	Status      string         `json:"status"`
	Summary     string         `json:"summary"`
	Outputs     map[string]any `json:"outputs"`
	NextSignals []string       `json:"next_signals"`
}

type Report struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id"`
	IssueID       string         `json:"issue_id"`
	Stage         string         `json:"stage"`
	Attempt       int            `json:"attempt"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	Outputs       map[string]any `json:"outputs"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     string         `json:"created_at"`
}

type StepLog struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Step      string `json:"step"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

func main() { os.Exit(run()) }

func run() int {
	contextPath := mustEnv("AUTODEV_STAGE_CONTEXT")
	resultPath := mustEnv("AUTODEV_STAGE_RESULT")
	reportPath := mustEnv("AUTODEV_STAGE_REPORT")
	statePath := strings.TrimSpace(os.Getenv("AUTODEV_STAGE_STATE"))

	result, report := execute(contextPath, statePath)
	if err := contracts.WriteFile(resultPath, contracts.StageResultSchema, result); err != nil {
		logmsg("write result: %v", err)
		return 2
	}
	if err := contracts.WriteFile(reportPath, contracts.StageReportSchema, report); err != nil {
		logmsg("write report: %v", err)
		return 2
	}
	return 0
}

func execute(contextPath, statePath string) (*Result, *Report) {
	attr := readFailureAttribution(contextPath)
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		msg := fmt.Sprintf("read %s: %v", contextPath, err)
		return failedResult(msg), failedReport(attr, msg)
	}
	contract := Contract{}
	if err := contracts.Unmarshal(raw, contracts.StageContextSchema, contextPath, &contract); err != nil {
		msg := err.Error()
		return failedResult(msg), failedReport(attr, msg)
	}
	if err := validateContract(contract); err != nil {
		msg := err.Error()
		return failedResult(msg), failedReport(attr, msg)
	}
	attr = attributionFromContract(contract)

	full := map[string]any{}
	if err := contracts.Unmarshal(raw, "", contextPath, &full); err != nil {
		msg := err.Error()
		return failedResult(msg), failedReport(attr, msg)
	}

	meta := map[string]any{
		"state_transitions": []map[string]any{},
		"step_log":          []StepLog{},
	}
	result := &Result{
		Status:      "succeeded",
		Summary:     or(contract.Stage.Runtime.Summary, "Stage "+contract.Stage.Name+" completed."),
		Outputs:     buildOutputs(contract, full),
		NextSignals: nonNil(contract.Stage.Runtime.Transitions.OnSuccess),
	}

	workDir, err := os.MkdirTemp("", "autodev-stage-"+contract.Attempt.ID+"-")
	if err != nil {
		msg := fmt.Sprintf("create work dir: %v", err)
		return failedResult(msg), failedReport(attr, msg)
	}
	defer os.RemoveAll(workDir)

	workingResult := workDir + "/result.json"
	workingMeta := workDir + "/metadata.json"

	recordTransition(meta, "load_contract", "execute_steps", result.Status, result.Summary)
	writeState(statePath, contract, result, meta, "load_contract", "execute_steps")

	for i, step := range contract.Stage.OperationPlan.Steps {
		label := or(step.Name, strings.Join(step.Command, " "))
		if err := contracts.WriteFile(workingResult, contracts.StageResultSchema, result); err != nil {
			msg := fmt.Sprintf("persist working result: %v", err)
			return failedResult(msg), failedReport(attr, msg)
		}
		if err := contracts.WriteFile(workingMeta, contracts.StageMetadataSchema, meta); err != nil {
			msg := fmt.Sprintf("persist working metadata: %v", err)
			return failedResult(msg), failedReport(attr, msg)
		}

		recordTransition(meta, label, label, result.Status, "running step")
		writeState(statePath, contract, result, meta, label, label)

		code, stdout, stderr := runCmd(step, map[string]string{
			"AUTODEV_STAGE_CONTEXT":          contextPath,
			"AUTODEV_STAGE_RESULT_WORKING":   workingResult,
			"AUTODEV_STAGE_METADATA_WORKING": workingMeta,
			"AUTODEV_STAGE_STEP_INDEX":       fmt.Sprintf("%d", i+1),
			"AUTODEV_STAGE_STEP_NAME":        label,
		})

		if err := reload(workingResult, contracts.StageResultSchema, result); err != nil {
			entry := StepLog{Timestamp: now(), Event: "failed", Step: label, Stdout: stdout, Stderr: err.Error()}
			entry.ExitCode = &code
			meta["step_log"] = appendStepLogs(meta["step_log"], entry)
			setFailed(result, contract.Stage.Runtime, err.Error())
			recordTransition(meta, label, "done", result.Status, result.Summary)
			writeState(statePath, contract, result, meta, label, "done")
			break
		}
		if err := reload(workingMeta, contracts.StageMetadataSchema, &meta); err != nil {
			entry := StepLog{Timestamp: now(), Event: "failed", Step: label, Stdout: stdout, Stderr: err.Error()}
			entry.ExitCode = &code
			meta["step_log"] = appendStepLogs(meta["step_log"], entry)
			setFailed(result, contract.Stage.Runtime, err.Error())
			recordTransition(meta, label, "done", result.Status, result.Summary)
			writeState(statePath, contract, result, meta, label, "done")
			break
		}

		entry := StepLog{
			Timestamp: now(),
			Step:      label,
			Stdout:    stdout,
			Stderr:    stderr,
		}
		entry.ExitCode = &code
		if code != 0 {
			entry.Event = "failed"
			meta["step_log"] = appendStepLogs(meta["step_log"], entry)
			setFailed(result, contract.Stage.Runtime, firstNonEmpty(strings.TrimSpace(stderr), strings.TrimSpace(stdout), fmt.Sprintf("exit code %d", code)))
			recordTransition(meta, label, "done", result.Status, result.Summary)
			writeState(statePath, contract, result, meta, label, "done")
			break
		}
		entry.Event = "succeeded"
		meta["step_log"] = appendStepLogs(meta["step_log"], entry)
		recordTransition(meta, label, "done", result.Status, result.Summary)
		writeState(statePath, contract, result, meta, label, "done")
	}

	if result.Status == "succeeded" {
		violations := checkCriteria(contract.Stage.Runtime.SuccessCriteria, *result, meta)
		result.Outputs["contract_validation"] = map[string]any{
			"passed":     len(violations) == 0,
			"violations": violations,
		}
		if len(violations) > 0 {
			setFailed(result, contract.Stage.Runtime, "Success criteria not satisfied.")
		}
	}
	recordTransition(meta, "validate_success_criteria", "done", result.Status, result.Summary)
	writeState(statePath, contract, result, meta, "validate_success_criteria", "done")
	return result, report(contract, result, meta)
}

func validateContract(c Contract) error {
	if c.Run.ID == "" || c.Attempt.ID == "" || c.Issue.ID == "" || c.Stage.Name == "" {
		return fmt.Errorf("contract missing required fields")
	}
	if len(c.WorkOrder) == 0 {
		return fmt.Errorf("contract missing work_order")
	}
	if len(c.Stage.OperationPlan.Steps) == 0 {
		return fmt.Errorf("operation_plan.steps is empty")
	}
	for i, step := range c.Stage.OperationPlan.Steps {
		if len(step.Command) == 0 {
			return fmt.Errorf("step %d: empty command", i+1)
		}
	}
	return nil
}

func report(c Contract, result *Result, meta map[string]any) *Report {
	return &Report{
		SchemaVersion: reportSchemaVersion,
		RunID:         c.Run.ID,
		IssueID:       c.Issue.ID,
		Stage:         c.Stage.Name,
		Attempt:       c.Attempt.Attempt,
		Status:        result.Status,
		Summary:       result.Summary,
		Outputs:       result.Outputs,
		Metadata:      meta,
		CreatedAt:     now(),
	}
}

func writeState(path string, c Contract, result *Result, meta map[string]any, state, next string) {
	if path == "" {
		return
	}
	_ = contracts.WriteFile(path, contracts.StageStateSchema, map[string]any{
		"schema_version": stateSchemaVersion,
		"timestamp":      now(),
		"run_id":         c.Run.ID,
		"attempt_id":     c.Attempt.ID,
		"issue_id":       c.Issue.ID,
		"stage":          c.Stage.Name,
		"state":          state,
		"next_state":     next,
		"current_step":   state,
		"status":         result.Status,
		"summary":        result.Summary,
		"last_transition": map[string]any{
			"timestamp": now(),
			"state":     state,
			"next":      next,
			"status":    result.Status,
			"summary":   result.Summary,
		},
		"metadata": meta,
	})
}

func reload(path, schema string, target any) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if err := contracts.ReadFile(path, schema, target); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

func runCmd(step Step, extra map[string]string) (int, string, string) {
	cmd := exec.Command(step.Command[0], step.Command[1:]...)
	if strings.TrimSpace(step.Cwd) != "" {
		cmd.Dir = step.Cwd
	}
	env := runtimeSubprocessEnv(extra)
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var stdout, stderr capWriter
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), stdout.String(), stderr.String()
		}
		return 127, "", err.Error()
	}
	return 0, stdout.String(), stderr.String()
}

func buildOutputs(c Contract, full map[string]any) map[string]any {
	outputs := map[string]any{
		"stage":       c.Stage.Name,
		"run_id":      c.Run.ID,
		"attempt_id":  c.Attempt.ID,
		"issue_id":    c.Issue.ID,
		"issue_title": c.Issue.Title,
		"timestamp":   now(),
	}
	for _, key := range []string{"work_order", "runtime_isolation", "resolved_secrets", "materialized_repos", "invariants", "pipeline_contract"} {
		if value, ok := full[key]; ok {
			outputs[key] = value
		}
	}
	return outputs
}

func checkCriteria(criteria struct {
	RequireSummary  bool     `json:"require_summary"`
	RequiredOutputs []string `json:"required_outputs"`
	RequiredMeta    []string `json:"required_report_metadata"`
}, result Result, meta map[string]any) []map[string]any {
	violations := make([]map[string]any, 0)
	if criteria.RequireSummary && strings.TrimSpace(result.Summary) == "" {
		violations = append(violations, map[string]any{"type": "missing_summary"})
	}
	for _, key := range criteria.RequiredOutputs {
		if _, ok := result.Outputs[key]; !ok {
			violations = append(violations, map[string]any{"type": "missing_output", "field": key})
		}
	}
	for _, key := range criteria.RequiredMeta {
		if blank(meta[key]) {
			violations = append(violations, map[string]any{"type": "missing_report_metadata", "field": key})
		}
	}
	return violations
}

func setFailed(result *Result, runtime Runtime, summary string) {
	result.Status = "failed"
	result.Summary = summary
	result.NextSignals = nonNil(runtime.Transitions.OnFailure)
}

func failedResult(summary string) *Result {
	return &Result{Status: "failed", Summary: summary, Outputs: map[string]any{}, NextSignals: []string{}}
}

func failedReport(attr failureAttribution, summary string) *Report {
	return &Report{
		SchemaVersion: reportSchemaVersion,
		RunID:         attr.RunID,
		IssueID:       attr.IssueID,
		Stage:         attr.Stage,
		Attempt:       attr.Attempt,
		Status:        "failed",
		Summary:       summary,
		Outputs:       map[string]any{},
		Metadata:      map[string]any{"state_transitions": []map[string]any{}, "step_log": []StepLog{}},
		CreatedAt:     now(),
	}
}

func recordTransition(meta map[string]any, state, next, status, summary string) {
	transitions, _ := meta["state_transitions"].([]map[string]any)
	transitions = append(transitions, map[string]any{
		"timestamp": now(),
		"state":     state,
		"next":      next,
		"status":    status,
		"summary":   summary,
	})
	meta["state_transitions"] = transitions
}

func appendStepLogs(current any, entry StepLog) []StepLog {
	if typed, ok := current.([]StepLog); ok {
		return append(typed, entry)
	}
	if raw, ok := current.([]any); ok {
		out := make([]StepLog, 0, len(raw)+1)
		for _, item := range raw {
			if step, ok := item.(map[string]any); ok {
				out = append(out, StepLog{
					Timestamp: asString(step["timestamp"]),
					Event:     asString(step["event"]),
					Step:      asString(step["step"]),
					Stdout:    asString(step["stdout"]),
					Stderr:    asString(step["stderr"]),
				})
			}
		}
		return append(out, entry)
	}
	return []StepLog{entry}
}

func mustEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		logmsg("missing required env: %s", key)
		os.Exit(2)
	}
	return value
}

func logmsg(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[autodev-stage] "+format+"\n", args...)
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

type failureAttribution struct {
	RunID   string
	IssueID string
	Stage   string
	Attempt int
}

func attributionFromContract(c Contract) failureAttribution {
	return failureAttribution{
		RunID:   firstNonEmpty(c.Run.ID, "unknown-run"),
		IssueID: firstNonEmpty(c.Issue.ID, "unknown-issue"),
		Stage:   firstNonEmpty(c.Stage.Name, "unknown-stage"),
		Attempt: c.Attempt.Attempt,
	}
}

func readFailureAttribution(path string) failureAttribution {
	attr := failureAttribution{
		RunID:   "unknown-run",
		IssueID: "unknown-issue",
		Stage:   "unknown-stage",
		Attempt: 0,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return attr
	}
	var partial struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
		Attempt struct {
			Attempt int `json:"attempt"`
		} `json:"attempt"`
		Stage struct {
			Name string `json:"name"`
		} `json:"stage"`
	}
	if err := contracts.Unmarshal(data, "", path, &partial); err != nil {
		return attr
	}
	attr.RunID = firstNonEmpty(partial.Run.ID, attr.RunID)
	attr.IssueID = firstNonEmpty(partial.Issue.ID, attr.IssueID)
	attr.Stage = firstNonEmpty(partial.Stage.Name, attr.Stage)
	attr.Attempt = partial.Attempt.Attempt
	return attr
}

func runtimeSubprocessEnv(extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TMPDIR=/tmp",
		"TEMP=/tmp",
		"TMP=/tmp",
		"LANG=C",
		"LC_ALL=C",
		"LC_CTYPE=C",
	}
	return env
}

func or(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func blank(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []map[string]any:
		return len(t) == 0
	case []StepLog:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

type capWriter struct {
	builder strings.Builder
	total   int
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.total < maxCapture {
		limit := min(len(p), maxCapture-w.total)
		w.builder.Write(p[:limit])
	}
	w.total += len(p)
	return len(p), nil
}

func (w *capWriter) String() string { return w.builder.String() }

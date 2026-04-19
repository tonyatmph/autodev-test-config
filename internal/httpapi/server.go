package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/controlplane"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/runner"
	"g7.mph.tech/mph-tech/autodev/internal/workorder"
)

type Server struct {
	service  *controlplane.Service
	resolver *runner.Resolver
	runtime  RuntimeConfig
}

type RuntimeConfig struct {
	Env              app.Env
	RootDir          string
	DataDir          string
	GitLabDir        string
	WorkOrderRepo    string
	Specs            []model.StageSpec
	PipelineCatalog  map[string]any
	LocalIssueImport bool
}

func New(service *controlplane.Service, runtime RuntimeConfig, resolver *runner.Resolver) *Server {
	return &Server{service: service, runtime: runtime, resolver: resolver}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/projection", s.handleProjection)
	mux.Handle("/assets/", http.StripPrefix("/assets/", uiHandler()))
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/enqueue", s.handleEnqueue)
	mux.HandleFunc("/reconcile", s.handleReconcile)
	mux.HandleFunc("/recover", s.handleRecover)
	mux.HandleFunc("/snapshot", s.handleSnapshot)
	mux.HandleFunc("/attempts/claim", s.handleClaim)
	mux.HandleFunc("/attempts/", s.handleAttemptActions)
	mux.HandleFunc("/api/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/issues/import", s.handleIssueImport)
	mux.HandleFunc("/api/issues/", s.handleIssueActions)
	mux.HandleFunc("/api/runs/", s.handleRunActions)
	mux.HandleFunc("/api/pipelines", s.handlePipelines)
	mux.HandleFunc("/api/v1/overview", s.handleAPIV1Overview)
	mux.HandleFunc("/api/v1/issues", s.handleAPIV1Issues)
	mux.HandleFunc("/api/v1/issues/", s.handleAPIV1IssueActions)
	mux.HandleFunc("/api/v1/work-orders", s.handleAPIV1WorkOrders)
	mux.HandleFunc("/api/v1/work-orders/", s.handleAPIV1WorkOrder)
	mux.HandleFunc("/api/v1/pipelines", s.handleAPIV1Pipelines)
	mux.HandleFunc("/api/v1/stages", s.handleAPIV1Stages)
	mux.HandleFunc("/api/v1/runs", s.handleAPIV1Runs)
	mux.HandleFunc("/api/v1/runs/", s.handleAPIV1RunActions)
	mux.HandleFunc("/api/enqueue", s.handleEnqueue)
	mux.HandleFunc("/api/reconcile", s.handleReconcile)
	mux.HandleFunc("/api/recover", s.handleRecover)
	mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	return mux
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/assets/index.html", http.StatusFound)
}

func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.service.EnqueueFromGitLab(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.service.Reconcile(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	maxAge := 30 * time.Second
	if err := s.service.RecoverStuckRuns(maxAge); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkerID      string   `json:"worker_id"`
		LeaseSeconds  int      `json:"lease_seconds"`
		AllowedStages []string `json:"allowed_stages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	attempt, spec, err := s.service.ClaimNext(req.WorkerID, time.Duration(req.LeaseSeconds)*time.Second, req.AllowedStages)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if attempt == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attempt": attempt,
		"spec":    spec,
	})
}

func (s *Server) handleAttemptActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/attempts/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	attemptID, action := parts[0], parts[1]

	switch action {
	case "heartbeat":
		var req struct {
			LeaseSeconds int            `json:"lease_seconds"`
			Summary      string         `json:"summary"`
			Metadata     map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.service.Heartbeat(attemptID, time.Duration(req.LeaseSeconds)*time.Second, req.Summary, req.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "complete":
		var req struct {
			Result    model.StageResult   `json:"result"`
			Artifacts []model.ArtifactRef `json:"artifacts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.service.Complete(attemptID, req.Result, req.Artifacts); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildDashboardPayload(state))
}

func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":     time.Now().UTC(),
		"pipeline_catalog": summarizePipelineCatalog(s.runtime.PipelineCatalog),
		"stage_catalog":    summarizeStageCatalog(s.runtime.Specs),
	})
}

func (s *Server) handleAPIV1Overview(w http.ResponseWriter, r *http.Request) {
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildDashboardPayload(state))
}

func (s *Server) handleAPIV1Issues(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state, err := s.service.Snapshot()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		issues, _ := buildDashboardPayload(state)["issues"]
		writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
	case http.MethodPost:
		s.handleIssueImport(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIV1IssueActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/issues/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		state, err := s.service.Snapshot()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tracked, ok := state.Issues[parts[0]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, tracked)
		return
	}
	if len(parts) == 2 {
		r.URL.Path = "/api/issues/" + strings.Join(parts, "/")
		s.handleIssueActions(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleAPIV1WorkOrders(w http.ResponseWriter, r *http.Request) {
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs := make([]model.RunRequest, 0, len(state.Runs))
	for _, run := range state.Runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].UpdatedAt.After(runs[j].UpdatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"work_orders": summarizeWorkOrders(state, runs)})
}

func (s *Server) handleAPIV1WorkOrder(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/work-orders/"), "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs := make([]model.RunRequest, 0)
	for _, run := range state.Runs {
		if run.WorkOrder.ID == id {
			runs = append(runs, run)
		}
	}
	if len(runs) == 0 {
		http.NotFound(w, r)
		return
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].UpdatedAt.After(runs[j].UpdatedAt) })
	summaries := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, summarizeRun(state, run))
	}
	workOrders := summarizeWorkOrders(state, runs)
	writeJSON(w, http.StatusOK, map[string]any{
		"work_order": workOrders[0],
		"runs":       summaries,
	})
}

func (s *Server) handleAPIV1Pipelines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, summarizePipelineCatalog(s.runtime.PipelineCatalog))
}

func (s *Server) handleAPIV1Stages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"stages": summarizeStageCatalog(s.runtime.Specs)})
}

func (s *Server) handleAPIV1Runs(w http.ResponseWriter, r *http.Request) {
	state, err := s.service.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs := make([]model.RunRequest, 0, len(state.Runs))
	for _, run := range state.Runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].UpdatedAt.After(runs[j].UpdatedAt) })
	summaries := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, summarizeRun(state, run))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":           summaries,
		"pipeline_board": summarizePipelineBoard(summaries),
	})
}

func (s *Server) handleAPIV1RunActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/runs/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		state, err := s.service.Snapshot()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		run, ok := state.Runs[parts[0]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, summarizeRun(state, run))
		return
	}
	if len(parts) >= 2 {
		r.URL.Path = "/api/runs/" + strings.Join(parts, "/")
		s.handleRunActions(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleIssueImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.runtime.LocalIssueImport {
		http.Error(w, "local issue import is only available with the filesystem GitLab adapter", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(s.runtime.GitLabDir) == "" {
		http.Error(w, "local issue import requires runtime gitlab dir", http.StatusBadRequest)
		return
	}
	var issue model.DeliveryIssue
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := contracts.Validate(contracts.IssueFileSchema, "dashboard issue import", raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(raw, &issue); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if issue.ID == "" {
		http.Error(w, "issue id is required", http.StatusBadRequest)
		return
	}
	issuesDir := filepath.Join(s.runtime.GitLabDir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := filepath.Join(issuesDir, issue.ID+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.service.EnqueueFromGitLab(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"issue_id": issue.ID,
		"path":     path,
	})
}

func (s *Server) handleIssueActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/issues/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	issueID, action := parts[0], parts[1]
	switch action {
	case "materialize":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		run, outputs, err := s.materializeIssue(issueID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"issue_id": issueID,
			"run_id":   run.ID,
			"outputs":  outputs,
		})
	case "reconcile":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.service.ReconcileIssue(issueID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleRunActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	runID, action := parts[0], parts[1]
	switch action {
	case "index":
		index, err := s.readRunIndex(runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, index)
	case "pipeline":
		if len(parts) != 3 {
			http.NotFound(w, r)
			return
		}
		payload, err := s.readPipelineArtifact(runID, parts[2])
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case "stages":
		if len(parts) != 4 || parts[3] != "report" {
			http.NotFound(w, r)
			return
		}
		report, err := s.readLatestStageReport(runID, parts[2])
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) materializeIssue(issueID string) (model.RunRequest, map[string]any, error) {
	if err := s.service.EnqueueFromGitLab(); err != nil {
		return model.RunRequest{}, nil, err
	}
	state, err := s.service.Snapshot()
	if err != nil {
		return model.RunRequest{}, nil, err
	}
	tracked, ok := state.Issues[issueID]
	if !ok {
		return model.RunRequest{}, nil, fmt.Errorf("issue %s not found", issueID)
	}
	run, err := s.service.EnsureRunForIssue(issueID)
	if err != nil {
		return model.RunRequest{}, nil, err
	}
	outputs, err := runner.MaterializePipelineArtifacts(context.Background(), s.runtime.Env, s.runtime.Specs, s.runtime.PipelineCatalog, run, tracked.DeliveryIssue)
	if err != nil {
		return model.RunRequest{}, nil, err
	}
	run, err = s.service.PersistPipelineArtifacts(run.ID, outputs)
	if err != nil {
		return model.RunRequest{}, nil, err
	}
	return run, outputs, nil
}

func buildDashboardPayload(state model.PersistedState) map[string]any {
	issues := make([]map[string]any, 0, len(state.Issues))
	runs := make([]model.RunRequest, 0, len(state.Runs))
	for _, run := range state.Runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})
	for _, tracked := range state.Issues {
		latest := latestRunForIssue(state, tracked.ID)
		entry := map[string]any{
			"id":         tracked.ID,
			"title":      tracked.Title,
			"issue_type": tracked.WorkOrder.IssueType,
			"labels":     tracked.Labels,
			"updated_at": tracked.UpdatedAt,
			"approval":   tracked.Approval,
		}
		if latest != nil {
			entry["latest_run"] = summarizeRun(state, *latest)
		}
		issues = append(issues, entry)
	}
	sort.Slice(issues, func(i, j int) bool {
		left, _ := issues[i]["updated_at"].(time.Time)
		right, _ := issues[j]["updated_at"].(time.Time)
		return left.After(right)
	})
	runSummaries := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		runSummaries = append(runSummaries, summarizeRun(state, run))
	}
	workOrders := summarizeWorkOrders(state, runs)
	return map[string]any{
		"generated_at":   time.Now().UTC(),
		"issues":         issues,
		"runs":           runSummaries,
		"work_orders":    workOrders,
		"pipeline_board": summarizePipelineBoard(runSummaries),
	}
}

func summarizeRun(state model.PersistedState, run model.RunRequest) map[string]any {
	attempts := make([]model.StageAttempt, 0)
	for _, attempt := range state.Attempts {
		if attempt.RunID == run.ID {
			attempts = append(attempts, attempt)
		}
	}
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].StartedAt == nil {
			return false
		}
		if attempts[j].StartedAt == nil {
			return true
		}
		return attempts[i].StartedAt.After(*attempts[j].StartedAt)
	})
	return map[string]any{
		"id":              run.ID,
		"issue_id":        run.IssueID,
		"work_order_id":   run.WorkOrder.ID,
		"work_order_name": run.WorkOrder.RequestedOutcome,
		"issue_type":      run.WorkOrder.IssueType,
		"pipeline_family": run.Metadata["pipeline_family"],
		"status":          run.Status,
		"updated_at":      run.UpdatedAt,
		"created_at":      run.CreatedAt,
		"metadata":        run.Metadata,
		"attempts":        attempts,
		"current_stage":   detectCurrentStage(run.Metadata),
		"stats":           run.Stats,
	}
}

func latestRunForIssue(state model.PersistedState, issueID string) *model.RunRequest {
	var latest *model.RunRequest
	for _, run := range state.Runs {
		if run.IssueID != issueID {
			continue
		}
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) {
			copyRun := run
			latest = &copyRun
		}
	}
	return latest
}

func (s *Server) readRunIndex(runID string) (map[string]any, error) {
	state, err := s.service.Snapshot()
	if err != nil {
		return nil, err
	}
	run, ok := state.Runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	reader := workorder.NewReader(s.runtime.WorkOrderRepo)
	indexPath := filepath.Join(s.runtime.WorkOrderRepo, "work-orders", sanitizePathSegment(run.WorkOrder.ID), "runs", sanitizePathSegment(run.ID), "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	_ = reader
	return payload, nil
}

func (s *Server) readLatestStageReport(runID, stage string) (map[string]any, error) {
	state, err := s.service.Snapshot()
	if err != nil {
		return nil, err
	}
	run, ok := state.Runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	reader := workorder.NewReader(s.runtime.WorkOrderRepo)
	attempt, err := reader.LatestAttempt(run, stage)
	if err != nil {
		return nil, err
	}
	return reader.ReadReport(run, *attempt)
}

func (s *Server) readPipelineArtifact(runID, artifact string) (map[string]any, error) {
	state, err := s.service.Snapshot()
	if err != nil {
		return nil, err
	}
	run, ok := state.Runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	allowed := map[string]bool{
		"pipeline_intent":         true,
		"policy_evaluation":       true,
		"pipeline_build_plan":     true,
		"pipeline_execution_plan": true,
	}
	if !allowed[artifact] {
		return nil, fmt.Errorf("unknown pipeline artifact %s", artifact)
	}
	path := filepath.Join(s.runtime.WorkOrderRepo, "work-orders", sanitizePathSegment(run.WorkOrder.ID), "runs", sanitizePathSegment(run.ID), "pipeline", artifact+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func summarizePipelineCatalog(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return map[string]any{
			"issue_types": []map[string]any{},
			"pipelines":   []map[string]any{},
			"testing":     []map[string]any{},
		}
	}
	summary := map[string]any{
		"issue_types": []map[string]any{},
		"pipelines":   []map[string]any{},
		"testing":     []map[string]any{},
	}
	if issueTypes, ok := raw["issue_types"].(map[string]any); ok {
		rows := make([]map[string]any, 0, len(issueTypes))
		for name, value := range issueTypes {
			entry, _ := value.(map[string]any)
			rows = append(rows, map[string]any{
				"name":               name,
				"family":             entry["family"],
				"preferred_pipeline": entry["preferred_pipeline"],
				"optimization_goals": entry["optimization_goals"],
			})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
		summary["issue_types"] = rows
	}
	if pipelines, ok := raw["pipelines"].(map[string]any); ok {
		rows := make([]map[string]any, 0, len(pipelines))
		for name, value := range pipelines {
			entry, _ := value.(map[string]any)
			rows = append(rows, map[string]any{
				"name":                 name,
				"family":               entry["family"],
				"accepted_issue_types": entry["accepted_issue_types"],
				"testing_policy":       entry["testing_policy"],
				"optimization_goals":   entry["optimization_goals"],
				"summary":              entry["summary"],
			})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
		summary["pipelines"] = rows
	}
	if testing, ok := raw["testing_policies"].(map[string]any); ok {
		rows := make([]map[string]any, 0, len(testing))
		for name, value := range testing {
			entry, _ := value.(map[string]any)
			rows = append(rows, map[string]any{
				"name":                name,
				"strategy":            entry["strategy"],
				"immutable":           entry["immutable"],
				"readable_by_agent":   entry["readable_by_agent"],
				"executable_by_agent": entry["executable_by_agent"],
				"inspection_points":   entry["inspection_points"],
			})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
		summary["testing"] = rows
	}
	return summary
}

func summarizeStageCatalog(specs []model.StageSpec) []map[string]any {
	rows := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		rows = append(rows, map[string]any{
			"name":             spec.Name,
			"queue_mode":       spec.Runtime.QueueMode,
			"environment":      spec.Runtime.Environment,
			"dependencies":     spec.Dependencies,
			"run_as":           spec.RunAsIdentity(),
			"write_as":         spec.WriteAsIdentity(),
			"materialize":      spec.ContainerConfig().Materialize,
			"entrypoint":       spec.Entrypoint,
			"report_stages":    spec.Runtime.ReportStages,
			"success_criteria": spec.Runtime.SuccessCriteria,
			"summary":          spec.Runtime.Summary,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
	return rows
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "..", "-").Replace(value)
}

func summarizeWorkOrders(state model.PersistedState, runs []model.RunRequest) []map[string]any {
	byID := map[string]map[string]any{}
	for _, run := range runs {
		id := run.WorkOrder.ID
		if strings.TrimSpace(id) == "" {
			continue
		}
		entry, ok := byID[id]
		if !ok {
			entry = map[string]any{
				"id":                id,
				"issue_type":        run.WorkOrder.IssueType,
				"requested_outcome": run.WorkOrder.RequestedOutcome,
				"pipeline_family":   run.Metadata["pipeline_family"],
				"run_ids":           []string{},
				"statuses":          []string{},
				"active_runs":       0,
				"latest_updated":    run.UpdatedAt,
			}
			byID[id] = entry
		}
		entry["run_ids"] = append(entry["run_ids"].([]string), run.ID)
		entry["statuses"] = append(entry["statuses"].([]string), run.Status)
		if run.Status == model.RunStatusActive || run.Status == model.RunStatusAwaitingApproval {
			entry["active_runs"] = entry["active_runs"].(int) + 1
		}
		if run.UpdatedAt.After(entry["latest_updated"].(time.Time)) {
			entry["latest_updated"] = run.UpdatedAt
		}
	}
	rows := make([]map[string]any, 0, len(byID))
	for _, row := range byID {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["latest_updated"].(time.Time).After(rows[j]["latest_updated"].(time.Time))
	})
	return rows
}

func summarizePipelineBoard(runSummaries []map[string]any) []map[string]any {
	byFamily := map[string]map[string]any{}
	for _, run := range runSummaries {
		family, _ := run["pipeline_family"].(string)
		if strings.TrimSpace(family) == "" {
			family = "unassigned"
		}
		entry, ok := byFamily[family]
		if !ok {
			entry = map[string]any{
				"pipeline_family": family,
				"runs":            []map[string]any{},
				"active_runs":     0,
			}
			byFamily[family] = entry
		}
		row := map[string]any{
			"run_id":        run["id"],
			"work_order_id": run["work_order_id"],
			"issue_type":    run["issue_type"],
			"status":        run["status"],
			"current_stage": run["current_stage"],
			"updated_at":    run["updated_at"],
		}
		entry["runs"] = append(entry["runs"].([]map[string]any), row)
		if run["status"] == model.RunStatusActive || run["status"] == model.RunStatusAwaitingApproval {
			entry["active_runs"] = entry["active_runs"].(int) + 1
		}
	}
	rows := make([]map[string]any, 0, len(byFamily))
	for _, row := range byFamily {
		sort.Slice(row["runs"].([]map[string]any), func(i, j int) bool {
			return row["runs"].([]map[string]any)[i]["updated_at"].(time.Time).After(row["runs"].([]map[string]any)[j]["updated_at"].(time.Time))
		})
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["pipeline_family"].(string) < rows[j]["pipeline_family"].(string)
	})
	return rows
}

func detectCurrentStage(metadata map[string]any) string {
	stageStates, _ := metadata["current_stage_states"].(map[string]any)
	currentStage := ""
	latestHeartbeat := time.Time{}
	for stage, raw := range stageStates {
		payload, _ := raw.(map[string]any)
		status, _ := payload["status"].(string)
		if status == model.AttemptStatusRunning {
			return stage
		}
		if updatedRaw, ok := payload["updated_at"].(string); ok {
			if ts, err := time.Parse(time.RFC3339Nano, updatedRaw); err == nil && ts.After(latestHeartbeat) {
				latestHeartbeat = ts
				currentStage = stage
			}
		}
	}
	return currentStage
}

func (s *Server) handleProjection(w http.ResponseWriter, r *http.Request) {
    goalName := r.URL.Query().Get("goal")
    if goalName == "" {
        http.Error(w, "missing goal parameter", http.StatusBadRequest)
        return
    }
    
    // Create goal
    goal := runner.Goal{
        Contract: runner.Contract{Name: goalName, Version: "1.0"},
        InputSHA: "SHA-INITIAL",
    }
    
    projection, err := s.resolver.ProjectCurrentState(r.Context(), goal)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    writeJSON(w, http.StatusOK, projection)
}

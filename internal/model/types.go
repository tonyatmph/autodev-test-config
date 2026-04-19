package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	IssueLabelRequested        = "delivery/requested"
	IssueLabelActive           = "delivery/active"
	IssueLabelBlocked          = "delivery/blocked"
	IssueLabelAwaitingApproval = "delivery/awaiting-approval"
	IssueLabelCompleted        = "delivery/completed"
	IssueLabelFailed           = "delivery/failed"

	RunStatusPending          = "pending"
	RunStatusActive           = "active"
	RunStatusAwaitingApproval = "awaiting_approval"
	RunStatusCompleted        = "completed"
	RunStatusFailed           = "failed"

	AttemptStatusPending   = "pending"
	AttemptStatusRunning   = "running"
	AttemptStatusSucceeded = "succeeded"
	AttemptStatusFailed    = "failed"
	AttemptStatusBlocked   = "blocked"
)

type ExecutionIdentity string

const (
	ExecutionIdentityAgent     ExecutionIdentity = "agent"
	ExecutionIdentityGenerator ExecutionIdentity = "generator"
	ExecutionIdentityGoverned  ExecutionIdentity = "governed"
)

func (i ExecutionIdentity) Valid() bool {
	switch i {
	case ExecutionIdentityAgent, ExecutionIdentityGenerator, ExecutionIdentityGoverned:
		return true
	default:
		return false
	}
}

type DeliveryIssue struct {
	ID               string         `json:"id"`
	ProjectID        string         `json:"project_id"`
	IID              int            `json:"iid"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	Labels           []string       `json:"labels"`
	WorkOrder        WorkOrder      `json:"work_order"`
	Target           DeliveryTarget `json:"target"`
	RequestedOutcome string         `json:"requested_outcome"`
	PolicyProfile    string         `json:"policy_profile"`
	PipelineTemplate string         `json:"pipeline_template"`
	Approval         ApprovalGate   `json:"approval"`
	MergeRequests    []string       `json:"merge_requests"`
	Artifacts        []ArtifactRef  `json:"artifacts"`
	Metadata         map[string]any `json:"metadata"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type DeliveryTarget struct {
	Name               string              `json:"name"`
	PrimaryComponent   string              `json:"primary_component"`
	SelectedComponents []string            `json:"selected_components"`
	DeployAsUnit       bool                `json:"deploy_as_unit"`
	Documentation      DocumentationPolicy `json:"documentation"`
	Journal            JournalTarget       `json:"journal"`
	ApplicationRepo    RepoTarget          `json:"application_repo"`
	Components         DeliveryComponents  `json:"components"`
	Environments       PromotionTargets    `json:"environments"`
	Release            ReleaseDefinition   `json:"release"`
}

type DocumentationPolicy struct {
	Required      bool     `json:"required"`
	DocsComponent string   `json:"docs_component"`
	RequiredKinds []string `json:"required_kinds,omitempty"`
}

type WorkOrder struct {
	ID               string               `json:"id"`
	SourceIssueID    string               `json:"source_issue_id"`
	IssueType        string               `json:"issue_type"`
	RequestedOutcome string               `json:"requested_outcome"`
	PolicyProfile    string               `json:"policy_profile"`
	PipelineTemplate string               `json:"pipeline_template"`
	IssuerAuthority  IssuerAuthority      `json:"issuer_authority"`
	Testing          TestingPolicy        `json:"testing"`
	Delivery         DeliveryTarget       `json:"delivery"`
	Translation      WorkOrderTranslation `json:"translation"`
}

type WorkOrderTranslation struct {
	Translator string   `json:"translator"`
	Version    string   `json:"version"`
	Status     string   `json:"status"`
	Warnings   []string `json:"warnings"`
}

type IssuerAuthority struct {
	CanCreatePipeline bool     `json:"can_create_pipeline"`
	Roles             []string `json:"roles,omitempty"`
}

type TestingPolicy struct {
	Strategy          string            `json:"strategy"`
	Immutable         bool              `json:"immutable"`
	ReadableByAgent   bool              `json:"readable_by_agent"`
	ExecutableByAgent bool              `json:"executable_by_agent"`
	InspectionPoints  []InspectionPoint `json:"inspection_points,omitempty"`
}

type InspectionPoint struct {
	Name              string `json:"name"`
	Category          string `json:"category"`
	Description       string `json:"description,omitempty"`
	Immutable         bool   `json:"immutable"`
	ReadableByAgent   bool   `json:"readable_by_agent"`
	ExecutableByAgent bool   `json:"executable_by_agent"`
}

type DeliveryComponents map[string]DeliveryComponent

type PathOwnershipRule struct {
	Identity    ExecutionIdentity `json:"identity"`
	Paths       []string          `json:"paths,omitempty"`
	Mutable     bool              `json:"mutable"`
	Description string            `json:"description,omitempty"`
}

type ComponentRepository struct {
	Repo      RepoTarget          `json:"repo"`
	Ownership []PathOwnershipRule `json:"ownership,omitempty"`
}

type DeliveryComponent struct {
	Name         string                `json:"name"`
	Kind         string                `json:"kind"`
	Deployable   bool                  `json:"deployable"`
	DependsOn    []string              `json:"depends_on,omitempty"`
	Repo         RepoTarget            `json:"repo"`
	Repositories []ComponentRepository `json:"repositories,omitempty"`
	Ownership    []PathOwnershipRule   `json:"ownership,omitempty"`
	Release      ReleaseDefinition     `json:"release"`
}

type JournalTarget struct {
	Name        string     `json:"name"`
	Repo        RepoTarget `json:"repo"`
	Path        string     `json:"path,omitempty"`
	Strategy    string     `json:"strategy"` // e.g. git
	Description string     `json:"description,omitempty"`
}

type RepoTarget struct {
	ProjectPath         string `json:"project_path"`
	DefaultBranch       string `json:"default_branch"`
	WorkingBranchPrefix string `json:"working_branch_prefix"`
	Ref                 string `json:"ref,omitempty"`
	MaterializationPath string `json:"materialization_path,omitempty"`
}

type GitOpsTarget struct {
	ProjectPath         string `json:"project_path"`
	Environment         string `json:"environment"`
	Path                string `json:"path"`
	PromotionBranch     string `json:"promotion_branch"`
	Cluster             string `json:"cluster"`
	Ref                 string `json:"ref,omitempty"`
	MaterializationPath string `json:"materialization_path,omitempty"`
}

type EnvironmentTarget struct {
	Name              string             `json:"name"`
	GitOpsRepo        GitOpsTarget       `json:"gitops_repo"`
	ApprovalRequired  bool               `json:"approval_required"`
	RolloutStrategy   string             `json:"rollout_strategy"`
	RuntimeSecretRefs []RuntimeSecretRef `json:"runtime_secret_refs"`
	RollbackPolicy    RollbackPolicy     `json:"rollback_policy"`
}

type PromotionTargets struct {
	Local EnvironmentTarget `json:"local"`
	Dev   EnvironmentTarget `json:"dev"`
	Prod  EnvironmentTarget `json:"prod"`
}

type ReleaseDefinition struct {
	Application    ApplicationRelease   `json:"application"`
	Infrastructure InfrastructureChange `json:"infrastructure"`
	Database       DatabaseChange       `json:"database"`
}

type ApplicationRelease struct {
	ArtifactName string `json:"artifact_name"`
	ImageRepo    string `json:"image_repo"`
}

type InfrastructureChange struct {
	Ref           string `json:"ref"`
	GitOpsOnly    bool   `json:"gitops_only"`
	TerraformRoot string `json:"terraform_root"`
}

type DatabaseChange struct {
	BundleRef        string `json:"bundle_ref"`
	Compatibility    string `json:"compatibility"`
	GitOpsManagedJob bool   `json:"gitops_managed_job"`
}

type ReleaseManifest struct {
	RunID          string                 `json:"run_id"`
	IssueID        string                 `json:"issue_id"`
	DeliveryName   string                 `json:"delivery_name"`
	DeployAsUnit   bool                   `json:"deploy_as_unit"`
	Components     []ComponentManifest    `json:"components"`
	Application    ApplicationArtifact    `json:"application"`
	Infrastructure InfrastructureArtifact `json:"infrastructure"`
	Database       DatabaseArtifact       `json:"database"`
	Promotions     []EnvironmentPromotion `json:"promotions"`
}

type PipelineIntent struct {
	SchemaVersion      string                `json:"schema_version"`
	RunID              string                `json:"run_id"`
	IssueID            string                `json:"issue_id"`
	WorkOrderID        string                `json:"work_order_id"`
	IssueType          string                `json:"issue_type"`
	PipelineTemplate   string                `json:"pipeline_template"`
	PipelineFamily     string                `json:"pipeline_family"`
	PipelineSelection  string                `json:"pipeline_selection"`
	AcceptedIssueTypes []string              `json:"accepted_issue_types"`
	OptimizationGoals  []string              `json:"optimization_goals,omitempty"`
	PolicyProfile      string                `json:"policy_profile"`
	DeliveryName       string                `json:"delivery_name"`
	RequestedOutcome   string                `json:"requested_outcome"`
	DeployAsUnit       bool                  `json:"deploy_as_unit"`
	SelectedComponents []string              `json:"selected_components"`
	OrderedComponents  []string              `json:"ordered_components"`
	Documentation      DocumentationPolicy   `json:"documentation"`
	Testing            TestingPolicy         `json:"testing"`
	Environments       []PipelineEnvironment `json:"environments"`
}

type PolicyEvaluation struct {
	SchemaVersion string                         `json:"schema_version"`
	RunID         string                         `json:"run_id"`
	IssueID       string                         `json:"issue_id"`
	WorkOrderID   string                         `json:"work_order_id"`
	PolicyProfile string                         `json:"policy_profile"`
	Outcome       string                         `json:"outcome"`
	PipelineScope []PolicyLayerDecision          `json:"pipeline_scope"`
	Hierarchy     []PolicyLayerDecision          `json:"hierarchy"`
	StageScope    map[string]StagePolicyDecision `json:"stage_scope"`
	StagePolicies []StagePolicyDecision          `json:"stage_policies"`
}

type PolicyLayerDecision struct {
	Layer   string         `json:"layer"`
	Status  string         `json:"status"`
	Summary string         `json:"summary"`
	Details map[string]any `json:"details,omitempty"`
}

type StagePolicyDecision struct {
	Stage       string              `json:"stage"`
	Environment string              `json:"environment,omitempty"`
	Blocked     bool                `json:"blocked"`
	Reason      string              `json:"reason,omitempty"`
	Details     map[string]any      `json:"details,omitempty"`
	Contract    StagePolicyContract `json:"contract"`
}

type StagePolicyContract struct {
	RunAs           string         `json:"run_as,omitempty"`
	WriteAs         string         `json:"write_as,omitempty"`
	Materialize     []string       `json:"materialize,omitempty"`
	RequiredOutputs []string       `json:"required_outputs,omitempty"`
	ReportStages    []string       `json:"report_stages,omitempty"`
	SuccessCriteria map[string]any `json:"success_criteria,omitempty"`
	ImageRef        string         `json:"image_ref,omitempty"`
	ImageDigest     string         `json:"image_digest,omitempty"`
	Container       map[string]any `json:"container,omitempty"`
	EntryPoint      []string       `json:"entrypoint,omitempty"`
}

type PipelineBuildPlan struct {
	SchemaVersion          string              `json:"schema_version"`
	RunID                  string              `json:"run_id"`
	IssueID                string              `json:"issue_id"`
	WorkOrderID            string              `json:"work_order_id"`
	IssueType              string              `json:"issue_type"`
	PipelineFamily         string              `json:"pipeline_family"`
	OptimizationGoals      []string            `json:"optimization_goals,omitempty"`
	DeliveryName           string              `json:"delivery_name"`
	Images                 []PipelineImagePlan `json:"images"`
}

type PipelineImagePlan struct {
	Stage       string         `json:"stage"`
	Image       string         `json:"image_ref"`
	Digest      string         `json:"image_digest,omitempty"`
	Container   StageContainer `json:"container"`
	Entrypoint  []string       `json:"entrypoint"`
	ToolingRepo ToolingRepo    `json:"tooling_repo"`
	PromptFile  string         `json:"prompt_file"`
}

type PipelineExecutionPlan struct {
	SchemaVersion          string                   `json:"schema_version"`
	RunID                  string                   `json:"run_id"`
	IssueID                string                   `json:"issue_id"`
	WorkOrderID            string                   `json:"work_order_id"`
	IssueType              string                   `json:"issue_type"`
	PipelineFamily         string                   `json:"pipeline_family"`
	PipelineSelection      string                   `json:"pipeline_selection"`
	OptimizationGoals      []string                 `json:"optimization_goals,omitempty"`
	Testing                TestingPolicy            `json:"testing"`
	DeliveryName           string                   `json:"delivery_name"`
	Checkpoint             string                   `json:"checkpoint_status,omitempty"`
	Stages                 []PipelineExecutionStage `json:"stages"`
}

type PipelineExecutionStage struct {
	Name            string                `json:"name"`
	Spec            StageSpec             `json:"spec,omitempty"`
	Dependencies    []string              `json:"dependencies"`
	Environment     string                `json:"environment,omitempty"`
	QueueMode       StageQueueMode        `json:"queue_mode"`
	Transitions     StageTransition       `json:"transitions"`
	Checkpoint      string                `json:"checkpoint_status,omitempty"`
	RunAs           ExecutionIdentity     `json:"run_as,omitempty"`
	WriteAs         ExecutionIdentity     `json:"write_as,omitempty"`
	Container       StageContainer        `json:"container"`
	SuccessCriteria StageSuccessCriteria  `json:"success_criteria"`
	OutputArtifacts []StageOutputArtifact `json:"output_artifacts"`
	ReportStages    []string              `json:"report_stages"`
	Image           string                `json:"image_ref"`
	ImageDigest     string                `json:"image_digest,omitempty"`
	Entrypoint      []string              `json:"entrypoint"`
}

type PipelineEnvironment struct {
	Name             string `json:"name"`
	ProjectPath      string `json:"project_path"`
	Path             string `json:"path"`
	Cluster          string `json:"cluster"`
	ApprovalRequired bool   `json:"approval_required"`
}

type ComponentManifest struct {
	Name           string                 `json:"name"`
	Kind           string                 `json:"kind"`
	Deployable     bool                   `json:"deployable"`
	DependsOn      []string               `json:"depends_on,omitempty"`
	Repo           RepoTarget             `json:"repo"`
	Source         SourceStamp            `json:"source"`
	Application    ApplicationArtifact    `json:"application"`
	Infrastructure InfrastructureArtifact `json:"infrastructure"`
	Database       DatabaseArtifact       `json:"database"`
}

type SourceStamp struct {
	ProjectPath   string `json:"project_path"`
	DefaultBranch string `json:"default_branch"`
	CommitSHA     string `json:"commit_sha"`
	TreeState     string `json:"tree_state"`
}

type RuntimeSecretRef struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Ref      string `json:"ref"`
}

type ApplicationArtifact struct {
	Name        string `json:"name"`
	ImageRepo   string `json:"image_repo"`
	ImageTag    string `json:"image_tag"`
	ImageDigest string `json:"image_digest"`
}

type InfrastructureArtifact struct {
	ChangeRef      string `json:"change_ref"`
	TerraformRoot  string `json:"terraform_root"`
	ExecutionModel string `json:"execution_model"`
}

type DatabaseArtifact struct {
	BundleRef      string `json:"bundle_ref"`
	Compatibility  string `json:"compatibility"`
	ExecutionModel string `json:"execution_model"`
}

type EnvironmentPromotion struct {
	Name              string             `json:"name"`
	ProjectPath       string             `json:"project_path"`
	Path              string             `json:"path"`
	Cluster           string             `json:"cluster"`
	PromotionBranch   string             `json:"promotion_branch"`
	ApprovalRequired  bool               `json:"approval_required"`
	RolloutStrategy   string             `json:"rollout_strategy"`
	RuntimeSecretRefs []RuntimeSecretRef `json:"runtime_secret_refs"`
	RollbackPolicy    RollbackPolicy     `json:"rollback_policy"`
	PreviousKnownGood KnownGoodState     `json:"previous_known_good"`
}

type RollbackPolicy struct {
	AppAutoRollback   bool `json:"app_auto_rollback"`
	InfraRollbackable bool `json:"infra_rollbackable"`
	DBRollbackable    bool `json:"db_rollbackable"`
}

type KnownGoodState struct {
	ApplicationDigest string `json:"application_digest"`
	GitOpsCommit      string `json:"gitops_commit"`
	InfrastructureRef string `json:"infrastructure_ref"`
	DatabaseBundleRef string `json:"database_bundle_ref"`
}

type ApprovalGate struct {
	Label      string     `json:"label"`
	Approved   bool       `json:"approved"`
	ApprovedBy string     `json:"approved_by"`
	ApprovedAt *time.Time `json:"approved_at"`
}

type ToolingRepo struct {
	URL string `json:"url"`
	Ref string `json:"ref"`
}

type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
	BackoffSecs int `json:"backoff_secs"`
}

type ArtifactPolicy struct {
	Required        []string `json:"required"`
	Retention       string   `json:"retention"`
	PublishToGitLab bool     `json:"publish_to_gitlab"`
	SignedSummaries bool     `json:"signed_summaries"`
	AttachManifests bool     `json:"attach_manifests"`
}

type RuntimeIsolationMode string

const (
	RuntimeIsolationModeShared    RuntimeIsolationMode = "shared-process"
	RuntimeIsolationModeHost      RuntimeIsolationMode = "host-user"
	RuntimeIsolationModeContainer RuntimeIsolationMode = "container-user"
)

type RuntimeUserSpec struct {
	Mode          RuntimeIsolationMode `json:"mode,omitempty"`
	OSUser        string               `json:"os_user,omitempty"`
	ContainerUser string               `json:"container_user,omitempty"`
	Enforcement   string               `json:"enforcement,omitempty"` // advisory|required
}

type StageSurface string

const (
	StageSurfaceWorkspace  StageSurface = "workspace"
	StageSurfaceComponents StageSurface = "components"
	StageSurfaceJournal    StageSurface = "journal"
	StageSurfaceGitOps     StageSurface = "gitops"
)

type StagePermissions struct {
	Writable    []string        `json:"writable,omitempty"`
	RepoControl []string        `json:"repo_control,omitempty"`
	Network     string          `json:"network,omitempty"`
	RuntimeUser RuntimeUserSpec `json:"runtime_user,omitempty"`
}

type StageContainer struct {
	RunAs       ExecutionIdentity `json:"run_as,omitempty"`
	WriteAs     ExecutionIdentity `json:"write_as,omitempty"`
	Permissions StagePermissions  `json:"permissions,omitempty"`
	Materialize []string          `json:"materialize,omitempty"`
}

type StageQueueMode string

const (
	StageQueueModeAuto      StageQueueMode = "auto"
	StageQueueModeTriggered StageQueueMode = "triggered"
)

type StageTransition struct {
	OnSuccess []string `json:"on_success,omitempty"`
	OnFailure []string `json:"on_failure,omitempty"`
}

type StageRollbackPlan struct {
	Stage  string `json:"stage,omitempty"`
	Policy string `json:"policy,omitempty"`
}

type StageSignalProfile struct {
	FailureSeverity string `json:"failure_severity,omitempty"`
}

type StageStatsProfile struct {
	Model             string  `json:"model,omitempty"`
	ToolCalls         int     `json:"tool_calls,omitempty"`
	ToolingUSD        float64 `json:"tooling_usd,omitempty"`
	ExpectedArtifacts int     `json:"expected_artifacts,omitempty"`
}

type StageOutputArtifact struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type StageSuccessCriteria struct {
	ResultStatus       string   `json:"result_status,omitempty"`
	RequireSummary     bool     `json:"require_summary,omitempty"`
	RequiredOutputs    []string `json:"required_outputs,omitempty"`
	RequiredReportMeta []string `json:"required_report_metadata,omitempty"`
}

type OperationStep struct {
	Name    string   `json:"name,omitempty"`
	Command []string `json:"command"`
	Cwd     string   `json:"cwd,omitempty"`
}

type OperationPlan struct {
	Steps []OperationStep `json:"steps"`
}

type StageRuntime struct {
	QueueMode        StageQueueMode        `json:"queue_mode,omitempty"`
	Environment      string                `json:"environment,omitempty"`
	Summary          string                `json:"summary,omitempty"`
	SuccessCriteria  StageSuccessCriteria  `json:"success_criteria,omitempty"`
	ReportStages     []string              `json:"report_stages,omitempty"`
	TriggerOnSuccess []string              `json:"trigger_on_success,omitempty"`
	TriggerOnFailure []string              `json:"trigger_on_failure,omitempty"`
	Transitions      StageTransition       `json:"transitions,omitempty"`
	CheckpointStatus string                `json:"checkpoint_status,omitempty"`
	Rollback         StageRollbackPlan     `json:"rollback,omitempty"`
	Signal           StageSignalProfile    `json:"signal,omitempty"`
	Stats            StageStatsProfile     `json:"stats,omitempty"`
	OutputArtifacts  []StageOutputArtifact `json:"output_artifacts,omitempty"`
}

type StageSpec struct {
	Name              string            `json:"name"`
	Operation         string            `json:"operation"`
	OperationConfig   map[string]any    `json:"operation_config,omitempty"`
	Runtime           StageRuntime      `json:"runtime"`
	Version           string            `json:"version"`
	ToolingRepo       ToolingRepo       `json:"tooling_repo"`
	PromptFile        string            `json:"prompt_file"`
	ExecutionIdentity ExecutionIdentity `json:"execution_identity,omitempty"`
	RunAs             ExecutionIdentity `json:"run_as,omitempty"`
	WriteAs           ExecutionIdentity `json:"write_as,omitempty"`
	Permissions       StagePermissions  `json:"permissions,omitempty"`
	Container         StageContainer    `json:"container,omitempty"`
	AllowedSecrets    []string          `json:"allowed_secrets"`
	GitLabScopes      []string          `json:"gitlab_scopes"`
	InputSchema       json.RawMessage   `json:"input_schema"`
	OutputSchema      json.RawMessage   `json:"output_schema"`
	ArtifactPolicy    ArtifactPolicy    `json:"artifact_policy"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
	RetryPolicy       RetryPolicy       `json:"retry_policy"`
	MaxParallelism    int               `json:"max_parallelism"`
	Dependencies      []string          `json:"dependencies"`
	ApprovalRequired  bool              `json:"approval_required"`
	Entrypoint        []string          `json:"entrypoint"`
}

func (s StageSpec) ContainerConfig() StageContainer {
	return s.Container
}

func (s StageSpec) RunAsIdentity() ExecutionIdentity {
	return s.ContainerConfig().RunAs
}

func (s StageSpec) WriteAsIdentity() ExecutionIdentity {
	return s.ContainerConfig().WriteAs
}

func (s StageSpec) MaterializedSurface(surface StageSurface) bool {
	return s.ContainerConfig().Materializes(surface)
}

func (s *StageSpec) Normalize() {
}

func (s StageSpec) OrchestrationPlan() (OperationPlan, error) {
	if strings.TrimSpace(s.Operation) != "orchestrate" {
		return OperationPlan{}, fmt.Errorf("stage %s does not declare orchestrated execution", s.Name)
	}
	if len(s.OperationConfig) == 0 {
		return OperationPlan{}, nil
	}
	var plan OperationPlan
	payload, err := json.Marshal(s.OperationConfig)
	if err != nil {
		return OperationPlan{}, err
	}
	if err := json.Unmarshal(payload, &plan); err != nil {
		return OperationPlan{}, err
	}
	return plan, nil
}

func (s StageSpec) QueueMode() StageQueueMode {
	return s.Runtime.QueueMode
}

func (s StageSpec) Environment() string {
	return strings.TrimSpace(s.Runtime.Environment)
}

func (s StageSpec) SummaryText() string {
	return strings.TrimSpace(s.Runtime.Summary)
}

func (s StageSpec) SuccessCriteriaContract() StageSuccessCriteria {
	return s.Runtime.SuccessCriteria
}

func (s StageSpec) NextStages() []string {
	return append([]string(nil), s.Runtime.Transitions.OnSuccess...)
}

func (s StageSpec) FailureStages() []string {
	return append([]string(nil), s.Runtime.Transitions.OnFailure...)
}

func (s StageSpec) ReportInputs() []string {
	return append([]string(nil), s.Runtime.ReportStages...)
}

func (s StageSpec) SignalFailureSeverity() string {
	return strings.TrimSpace(s.Runtime.Signal.FailureSeverity)
}

func (s StageSpec) StatsModel() string {
	return strings.TrimSpace(s.Runtime.Stats.Model)
}

func (s StageSpec) StatsToolCalls() int {
	return s.Runtime.Stats.ToolCalls
}

func (s StageSpec) StatsToolingUSD() float64 {
	return s.Runtime.Stats.ToolingUSD
}

func (s StageSpec) AutoQueue() bool {
	return s.QueueMode() != StageQueueModeTriggered
}

func (s StageSpec) ApprovalCheckpoint() bool {
	return strings.TrimSpace(s.Runtime.CheckpointStatus) == RunStatusAwaitingApproval
}

func (s StageSpec) CompletionCheckpoint() bool {
	return strings.TrimSpace(s.Runtime.CheckpointStatus) == RunStatusCompleted
}

func (s StageSpec) FailureCheckpoint() bool {
	return strings.TrimSpace(s.Runtime.CheckpointStatus) == RunStatusFailed
}

func (p StagePermissions) AllowsWritable(surface StageSurface) bool {
	return allowsSurface(p.Writable, surface)
}

func (p StagePermissions) AllowsRepoControl(surface StageSurface) bool {
	return allowsSurface(p.RepoControl, surface)
}

func (c StageContainer) Materializes(surface StageSurface) bool {
	return allowsSurface(c.Materialize, surface)
}

func allowsSurface(values []string, surface StageSurface) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), string(surface)) {
			return true
		}
	}
	return false
}

type RunRequest struct {
	ID               string         `json:"id"`
	IssueID          string         `json:"issue_id"`
	WorkOrder        WorkOrder      `json:"work_order"`
	PipelineTemplate string         `json:"pipeline_template"`
	Target           DeliveryTarget `json:"target"`
	Status           string         `json:"status"`
	Stats            RunStats       `json:"stats"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Metadata         map[string]any `json:"metadata"`
}

type StageAttempt struct {
	ID             string         `json:"id"`
	RunID          string         `json:"run_id"`
	Stage          string         `json:"stage"`
	Attempt        int            `json:"attempt"`
	Status         string         `json:"status"`
	WorkerID       string         `json:"worker_id"`
	LeaseExpiresAt *time.Time     `json:"lease_expires_at"`
	StartedAt      *time.Time     `json:"started_at"`
	LastHeartbeat  *time.Time     `json:"last_heartbeat"`
	FinishedAt     *time.Time     `json:"finished_at"`
	Result         *StageResult   `json:"result"`
	Stats          AttemptStats   `json:"stats"`
	Artifacts      []ArtifactRef  `json:"artifacts"`
	Metadata       map[string]any `json:"metadata"`
}

type ArtifactManifest struct {
	RunID     string        `json:"run_id"`
	Stage     string        `json:"stage"`
	Artifacts []ArtifactRef `json:"artifacts"`
}

type ArtifactRef struct {
	Name            string         `json:"name"`
	URI             string         `json:"uri"`
	Digest          string         `json:"digest"`
	ProducerStage   string         `json:"producer_stage"`
	RetentionPolicy string         `json:"retention_policy"`
	Metadata        map[string]any `json:"metadata"`
}

type StageResult struct {
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	Outputs     json.RawMessage `json:"outputs"`
	NextSignals []string        `json:"next_signals"`
	Stats       AttemptStats    `json:"stats"`
}

// StageReportSchema captures a typed view of a completed stage attempt.
type StageReportSchema struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id"`
	IssueID       string         `json:"issue_id"`
	Stage         string         `json:"stage"`
	Attempt       int            `json:"attempt"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	Stats         AttemptStats   `json:"stats"`
	Artifacts     []ArtifactRef  `json:"artifacts"`
	Outputs       any            `json:"outputs,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

const StageReportSchemaVersion = "autodev-stage-report-v1"

type AttemptStats struct {
	Stage         string           `json:"stage"`
	WorkerID      string           `json:"worker_id,omitempty"`
	DurationMS    int64            `json:"duration_ms"`
	Substages     []SubstageTiming `json:"substages"`
	Cost          CostBreakdown    `json:"cost"`
	Usage         UsageMetrics     `json:"usage"`
	EvidenceCount int              `json:"evidence_count"`
	ArtifactCount int              `json:"artifact_count"`
}

type SubstageTiming struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
}

type CostBreakdown struct {
	Currency   string  `json:"currency"`
	ComputeUSD float64 `json:"compute_usd"`
	ModelUSD   float64 `json:"model_usd"`
	ToolingUSD float64 `json:"tooling_usd"`
	StorageUSD float64 `json:"storage_usd"`
	TotalUSD   float64 `json:"total_usd"`
}

type UsageMetrics struct {
	Model             string `json:"model"`
	InputTokens       int    `json:"input_tokens"`
	OutputTokens      int    `json:"output_tokens"`
	CachedInputTokens int    `json:"cached_input_tokens"`
	ToolCalls         int    `json:"tool_calls"`
	PromptBytes       int    `json:"prompt_bytes"`
	OutputBytes       int    `json:"output_bytes"`
}

type RunStats struct {
	Currency          string        `json:"currency"`
	TotalCostUSD      float64       `json:"total_cost_usd"`
	TotalDurationMS   int64         `json:"total_duration_ms"`
	CompletedAttempts int           `json:"completed_attempts"`
	FailedAttempts    int           `json:"failed_attempts"`
	BlockedAttempts   int           `json:"blocked_attempts"`
	ArtifactCount     int           `json:"artifact_count"`
	StageCount        int           `json:"stage_count"`
	Stages            []StageTotals `json:"stages"`
}

type StageTotals struct {
	Stage             string          `json:"stage"`
	Attempts          int             `json:"attempts"`
	CompletedAttempts int             `json:"completed_attempts"`
	FailedAttempts    int             `json:"failed_attempts"`
	BlockedAttempts   int             `json:"blocked_attempts"`
	DurationMS        int64           `json:"duration_ms"`
	ArtifactCount     int             `json:"artifact_count"`
	TotalCostUSD      float64         `json:"total_cost_usd"`
	Substages         []SubstageTotal `json:"substages"`
}

type SubstageTotal struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
}

type IssueComment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type TrackedIssue struct {
	DeliveryIssue
	Comments []IssueComment `json:"comments"`
}

type PersistedState struct {
	Issues   map[string]TrackedIssue `json:"issues"`
	Runs     map[string]RunRequest   `json:"runs"`
	Attempts map[string]StageAttempt `json:"attempts"`
	Counters map[string]int          `json:"counters"`
}

func (i DeliveryIssue) CanonicalWorkOrder() WorkOrder {
	return i.WorkOrder
}

func (i DeliveryIssue) DeliveryTarget() DeliveryTarget {
	return i.CanonicalWorkOrder().Delivery
}

func (r RunRequest) CanonicalWorkOrder() WorkOrder {
	return r.WorkOrder
}

func (r RunRequest) DeliveryTarget() DeliveryTarget {
	return r.CanonicalWorkOrder().Delivery
}

func (r RunRequest) RepoScope() string {
	return r.DeliveryTarget().ApplicationRepo.ProjectPath
}

func (r RunRequest) ServiceScope() string {
	projectPath := strings.TrimSpace(r.RepoScope())
	if projectPath == "" {
		return ""
	}
	parts := strings.Split(projectPath, "/")
	return parts[len(parts)-1]
}

func (r RunRequest) ReleaseDefinition() ReleaseDefinition {
	return r.DeliveryTarget().Release
}

func (r RunRequest) EnvironmentTarget(name string) EnvironmentTarget {
	target := r.DeliveryTarget()
	switch name {
	case "local":
		return target.Environments.Local
	case "dev":
		return target.Environments.Dev
	case "prod":
		return target.Environments.Prod
	default:
		return EnvironmentTarget{}
	}
}

func promotionTargetsEmpty(targets PromotionTargets) bool {
	return environmentTargetEmpty(targets.Local) && environmentTargetEmpty(targets.Dev) && environmentTargetEmpty(targets.Prod)
}

func environmentTargetByName(targets PromotionTargets, name string) EnvironmentTarget {
	switch name {
	case "local":
		return targets.Local
	case "dev":
		return targets.Dev
	case "prod":
		return targets.Prod
	default:
		return EnvironmentTarget{}
	}
}

func environmentTargetEmpty(target EnvironmentTarget) bool {
	return target.Name == "" &&
		target.GitOpsRepo.ProjectPath == "" &&
		target.GitOpsRepo.Environment == "" &&
		target.GitOpsRepo.Path == "" &&
		target.GitOpsRepo.PromotionBranch == "" &&
		target.GitOpsRepo.Cluster == "" &&
		target.GitOpsRepo.Ref == "" &&
		!target.ApprovalRequired &&
		target.RolloutStrategy == "" &&
		len(target.RuntimeSecretRefs) == 0 &&
		target.RollbackPolicy == (RollbackPolicy{})
}

func documentationPolicyEmpty(policy DocumentationPolicy) bool {
	return !policy.Required && policy.DocsComponent == "" && len(policy.RequiredKinds) == 0
}

func journalTargetEmpty(target JournalTarget) bool {
	return target.Name == "" &&
		target.Repo.ProjectPath == "" &&
		target.Repo.DefaultBranch == "" &&
		target.Repo.WorkingBranchPrefix == "" &&
		target.Repo.Ref == "" &&
		target.Path == "" &&
		target.Strategy == "" &&
		target.Description == ""
}

func detectPrimaryComponent(target DeliveryTarget) string {
	switch {
	case target.PrimaryComponent != "":
		return target.PrimaryComponent
	case len(target.SelectedComponents) > 0:
		return target.SelectedComponents[0]
	case len(target.Components) > 0:
		preferred := []string{"api", "console", "mobile", "config", "prompts"}
		for _, name := range preferred {
			if _, ok := target.Components[name]; ok {
				return name
			}
		}
		names := make([]string, 0, len(target.Components))
		for name := range target.Components {
			names = append(names, name)
		}
		slices.Sort(names)
		return names[0]
	default:
		return ""
	}
}

func (t DeliveryTarget) Component(name string) (DeliveryComponent, bool) {
	if t.Components == nil {
		return DeliveryComponent{}, false
	}
	component, ok := t.Components[name]
	if ok {
		return component, true
	}
	return DeliveryComponent{}, false
}

func (c DeliveryComponent) AllRepositories() []ComponentRepository {
	if len(c.Repositories) == 0 {
		return []ComponentRepository{
			{Repo: c.Repo},
		}
	}
	return c.Repositories
}

func (c DeliveryComponent) OwnershipRulesFor(identity ExecutionIdentity) []PathOwnershipRule {
	var rules []PathOwnershipRule
	for _, rule := range c.Ownership {
		if rule.Identity == identity {
			rules = append(rules, rule)
		}
	}
	for _, repo := range c.AllRepositories() {
		rules = append(rules, repo.OwnershipRulesFor(identity)...)
	}
	return rules
}

func (r ComponentRepository) OwnershipRulesFor(identity ExecutionIdentity) []PathOwnershipRule {
	var rules []PathOwnershipRule
	for _, rule := range r.Ownership {
		if rule.Identity == identity {
			rules = append(rules, rule)
		}
	}
	return rules
}

func (t DeliveryTarget) OwnershipRulesFor(identity ExecutionIdentity) []PathOwnershipRule {
	var rules []PathOwnershipRule
	for _, component := range t.SelectedDeliveryComponents() {
		if component.Name == "" {
			continue
		}
		rules = append(rules, component.OwnershipRulesFor(identity)...)
	}
	return rules
}

func (t DeliveryTarget) HasOwnershipRules(identity ExecutionIdentity) bool {
	for _, component := range t.SelectedDeliveryComponents() {
		if len(component.OwnershipRulesFor(identity)) > 0 {
			return true
		}
	}
	return false
}

func (t DeliveryTarget) SupportsExecutionIdentity(identity ExecutionIdentity, environment string) bool {
	return t.SupportsStageSurface(identity, StageSurfaceComponents, environment) ||
		t.SupportsStageSurface(identity, StageSurfaceJournal, environment) ||
		t.SupportsStageSurface(identity, StageSurfaceGitOps, environment)
}

func (t DeliveryTarget) SupportsStageSurface(identity ExecutionIdentity, surface StageSurface, environment string) bool {
	switch surface {
	case StageSurfaceWorkspace:
		return true
	case StageSurfaceComponents:
		return t.HasOwnershipRules(identity)
	case StageSurfaceJournal:
		journal := t.JournalOrDefault()
		return journal.Strategy == "git" && journal.Repo.ProjectPath != ""
	case StageSurfaceGitOps:
		target := environmentTargetByName(t.Environments, environment)
		return target.GitOpsRepo.ProjectPath != "" && target.GitOpsRepo.Path != ""
	default:
		return false
	}
}

func (t DeliveryTarget) SelectedDeliveryComponents() []DeliveryComponent {
	names := t.SelectedComponentNames()
	out := make([]DeliveryComponent, 0, len(names))
	for _, name := range names {
		if component, ok := t.Component(name); ok {
			out = append(out, component)
		}
	}
	return out
}

func (t DeliveryTarget) SelectedComponentNames() []string {
	names := append([]string(nil), t.SelectedComponents...)
	if len(names) == 0 && t.PrimaryComponent != "" {
		names = []string{t.PrimaryComponent}
	}
	if len(names) == 0 {
		names = make([]string, 0, len(t.Components))
		for name := range t.Components {
			names = append(names, name)
		}
		slices.Sort(names)
	}
	return names
}

func (t DeliveryTarget) HasSelectedComponent(name string) bool {
	for _, selected := range t.SelectedComponentNames() {
		if selected == name {
			return true
		}
	}
	return false
}

func (t DeliveryTarget) DocumentationStatus() (required bool, component string, satisfied bool, reason string) {
	policy := t.Documentation
	if !policy.Required {
		return false, "", true, ""
	}
	component = policy.DocsComponent
	if component == "" {
		return true, "", false, "documentation policy requires an explicit docs_component in the delivery contract"
	}
	if _, ok := t.Component(component); !ok {
		return true, component, false, "documentation policy requires a docs surface, but no docs component is defined in the delivery object"
	}
	if !t.HasSelectedComponent(component) {
		return true, component, false, "documentation policy requires the docs surface to be part of the selected delivery components"
	}
	return true, component, true, ""
}

func (t DeliveryTarget) JournalOrDefault() JournalTarget {
	return t.Journal
}

func (t DeliveryTarget) OrderedSelectedComponentNames() []string {
	selected := t.SelectedComponentNames()
	if len(selected) <= 1 {
		return selected
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		selectedSet[name] = struct{}{}
	}
	inDegree := make(map[string]int, len(selected))
	dependents := make(map[string][]string, len(selected))
	for _, name := range selected {
		component, ok := t.Component(name)
		if !ok {
			continue
		}
		for _, dep := range component.DependsOn {
			if _, ok := selectedSet[dep]; !ok {
				continue
			}
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}
	queue := make([]string, 0, len(selected))
	for _, name := range selected {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	slices.Sort(queue)
	ordered := make([]string, 0, len(selected))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ordered = append(ordered, current)
		for _, dep := range dependents[current] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				slices.Sort(queue)
			}
		}
	}
	if len(ordered) != len(selected) {
		return selected
	}
	return ordered
}

func (t DeliveryTarget) EnvironmentTarget(name string) EnvironmentTarget {
	return environmentTargetByName(t.Environments, name)
}

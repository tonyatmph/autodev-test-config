package agent

import (
	"context"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

// Invocation is the bounded envelope passed from a stage runtime to an agent.
// The runtime still owns orchestration, secrets, and side effects.
type Invocation struct {
	Stage      string              `json:"stage"`
	RunID      string              `json:"run_id"`
	AttemptID  string              `json:"attempt_id"`
	IssueID    string              `json:"issue_id"`
	IssueTitle string              `json:"issue_title"`
	WorkOrder  model.WorkOrder     `json:"work_order"`
	Inputs     map[string]any      `json:"inputs,omitempty"`
	Policy     map[string]any      `json:"policy,omitempty"`
	Artifacts  []model.ArtifactRef `json:"artifacts,omitempty"`
	Invariants map[string]any      `json:"invariants,omitempty"`
}

// Response is the structured agent result that stage handlers may consume
// before deterministic wrappers apply any side effects.
type Response struct {
	Summary string         `json:"summary,omitempty"`
	Outputs map[string]any `json:"outputs,omitempty"`
}

type Driver interface {
	Run(context.Context, Invocation) (Response, error)
}

// NoopDriver is the default agent plane implementation until concrete stage
// handlers opt into model-backed reasoning.
type NoopDriver struct{}

func (NoopDriver) Run(context.Context, Invocation) (Response, error) {
	return Response{}, nil
}

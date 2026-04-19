package policy

import (
	"fmt"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type StageDecision struct {
	Blocked bool           `json:"blocked"`
	Reason  string         `json:"reason,omitempty"`
	Checks  map[string]any `json:"checks,omitempty"`
}

func EvaluateStage(spec model.StageSpec, run model.RunRequest, issue model.DeliveryIssue) StageDecision {
	decision := StageDecision{
		Checks: map[string]any{},
	}

	container := spec.ContainerConfig()
	runAs := container.RunAs
	writeAs := container.WriteAs
	environment := spec.Environment()
	if runAs != "" || writeAs != "" {
		identityCheck := map[string]any{
			"run_as":               runAs,
			"write_as":             writeAs,
			"run_as_valid":         runAs == "" || runAs.Valid(),
			"write_as_valid":       writeAs == "" || writeAs.Valid(),
			"write_surfaces_valid": true,
			"repo_control_valid":   true,
		}
		decision.Checks["execution_identity"] = identityCheck
		if runAs != "" && !runAs.Valid() {
			decision.Blocked = true
			decision.Reason = fmt.Sprintf("Stage %s declares unknown run_as identity %q", spec.Name, runAs)
			return decision
		}
		if writeAs != "" && !writeAs.Valid() {
			decision.Blocked = true
			decision.Reason = fmt.Sprintf("Stage %s declares unknown write_as identity %q", spec.Name, writeAs)
			return decision
		}
		for _, surface := range container.Permissions.Writable {
			if !run.DeliveryTarget().SupportsStageSurface(writeAs, model.StageSurface(surface), environment) {
				identityCheck["write_surfaces_valid"] = false
				decision.Blocked = true
				decision.Reason = fmt.Sprintf("Stage %s requires write access to %s as %s, but the delivery target provides none", spec.Name, surface, writeAs)
				return decision
			}
		}
		for _, surface := range container.Permissions.RepoControl {
			if !run.DeliveryTarget().SupportsStageSurface(writeAs, model.StageSurface(surface), environment) {
				identityCheck["repo_control_valid"] = false
				decision.Blocked = true
				decision.Reason = fmt.Sprintf("Stage %s requires repo control for %s as %s, but the delivery target provides none", spec.Name, surface, writeAs)
				return decision
			}
		}
	}
	decision.Checks["permissions"] = map[string]any{
		"writable":     append([]string(nil), container.Permissions.Writable...),
		"repo_control": append([]string(nil), container.Permissions.RepoControl...),
		"network":      container.Permissions.Network,
		"runtime_user": container.Permissions.RuntimeUser,
	}

	requiredDocs, docsComponent, docsSatisfied, docsReason := run.DeliveryTarget().DocumentationStatus()
	decision.Checks["documentation"] = map[string]any{
		"required":       requiredDocs,
		"docs_component": docsComponent,
		"satisfied":      docsSatisfied,
		"reason":         docsReason,
	}
	if requiresDocumentationSurface(spec) && requiredDocs && !docsSatisfied {
		decision.Blocked = true
		decision.Reason = docsReason
		return decision
	}

	approved := issue.Approval.Approved
	decision.Checks["approval"] = map[string]any{
		"required": spec.ApprovalRequired,
		"approved": approved,
		"label":    issue.Approval.Label,
	}
	if spec.ApprovalRequired && !approved {
		decision.Blocked = true
		decision.Reason = fmt.Sprintf("Stage %s is blocked until GitLab approval is present.", spec.Name)
	}

	return decision
}

func requiresDocumentationSurface(spec model.StageSpec) bool {
	for _, stage := range spec.ReportInputs() {
		if stage != "" {
			return true
		}
	}
	return false
}

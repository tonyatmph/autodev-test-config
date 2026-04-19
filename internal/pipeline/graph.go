package pipeline

import (
	"sort"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func SpecsInOrder(specs []model.StageSpec) []model.StageSpec {
	index := make(map[string]model.StageSpec, len(specs))
	indegree := make(map[string]int, len(specs))
	dependents := make(map[string][]string, len(specs))
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		index[spec.Name] = spec
		if _, ok := indegree[spec.Name]; !ok {
			indegree[spec.Name] = 0
		}
		names = append(names, spec.Name)
	}
	for _, spec := range specs {
		for _, dep := range spec.Dependencies {
			if _, ok := index[dep]; !ok {
				continue
			}
			indegree[spec.Name]++
			dependents[dep] = append(dependents[dep], spec.Name)
		}
	}
	sort.Strings(names)
	ready := make([]string, 0, len(names))
	for _, name := range names {
		if indegree[name] == 0 {
			ready = append(ready, name)
		}
	}
	ordered := make([]model.StageSpec, 0, len(specs))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, index[name])
		children := append([]string(nil), dependents[name]...)
		sort.Strings(children)
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(ordered) == len(specs) {
		return ordered
	}
	seen := make(map[string]struct{}, len(ordered))
	for _, spec := range ordered {
		seen[spec.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		ordered = append(ordered, index[name])
	}
	return ordered
}

func rollbackEligible(spec model.StageSpec, target model.DeliveryTarget, byStage map[string]model.StageAttempt) bool {
	rollback := spec.Runtime.Rollback
	if rollback.Stage == "" {
		return false
	}
	if rollback.Policy != "app_auto_rollback" {
		return false
	}
	if !rollbackPolicyEnabled(target, spec.Environment(), rollback.Policy) {
		return false
	}
	if observe, ok := byStage[rollback.Stage]; ok && observe.Status == model.AttemptStatusSucceeded {
		return false
	}
	return true
}

func rollbackPolicyEnabled(target model.DeliveryTarget, environment, policy string) bool {
	switch policy {
	case "app_auto_rollback":
		return target.EnvironmentTarget(environment).RollbackPolicy.AppAutoRollback
	default:
		return false
	}
}

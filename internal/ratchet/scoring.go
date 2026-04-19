package ratchet

import (
	"math"
	"slices"
	"sort"
	"time"
)

func RankForStage(now time.Time, invariants []ActiveInvariant, stats map[int64]InvariantStageStat, req RetrievalRequest) []RankedInvariant {
	ranked := make([]RankedInvariant, 0, len(invariants))
	for _, invariant := range invariants {
		if !scopeMatches(invariant, req) {
			continue
		}
		stageStat, ok := stats[invariant.ID]
		var stageStatPtr *InvariantStageStat
		score := severityScore(invariant.Severity) + enforcementScore(invariant.EnforcementMode)
		if ok {
			stageStatCopy := stageStat
			stageStatPtr = &stageStatCopy
			score += float64(stageStat.OccurrenceCount) * 1.5
			score += float64(stageStat.RollbackCount) * 6.0
			score += float64(stageStat.BlockCount) * 4.0
			if !stageStat.LastSeenAt.IsZero() {
				days := now.Sub(stageStat.LastSeenAt).Hours() / 24
				score += math.Max(0, 10-days)
			}
		}
		ranked = append(ranked, RankedInvariant{
			ActiveInvariant: invariant,
			StageStat:       stageStatPtr,
			Score:           score,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].ID < ranked[j].ID
		}
		return ranked[i].Score > ranked[j].Score
	})
	if req.Limit > 0 && len(ranked) > req.Limit {
		ranked = ranked[:req.Limit]
	}
	return ranked
}

func scopeMatches(invariant ActiveInvariant, req RetrievalRequest) bool {
	switch invariant.ScopeType {
	case "global":
		return true
	case "repo":
		return invariant.ScopeKey == req.RepoScope
	case "environment":
		return invariant.ScopeKey == req.Environment
	case "service":
		return invariant.ScopeKey == req.ServiceScope
	default:
		return false
	}
}

func severityScore(severity string) float64 {
	switch severity {
	case "critical":
		return 30
	case "high":
		return 20
	case "medium":
		return 10
	case "low":
		return 5
	default:
		return 1
	}
}

func enforcementScore(mode string) float64 {
	switch mode {
	case "block":
		return 25
	case "warn":
		return 10
	case "inform":
		return 3
	default:
		return 0
	}
}

func shouldPropose(cluster FindingCluster) (bool, string) {
	switch cluster.HighestSeverity {
	case "critical":
		return true, "severity_threshold"
	}
	if cluster.OccurrenceCount >= 3 && cluster.DistinctRunCount >= 3 {
		return true, "frequency_threshold"
	}
	if cluster.OccurrenceCount >= 2 && cluster.ScopeType == "global" {
		return true, "global_frequency_threshold"
	}
	return false, ""
}

func statementForCluster(cluster FindingCluster) string {
	return cluster.Title
}

func rationaleForCluster(cluster FindingCluster, reason string) string {
	return "Proposed from recurring finding cluster via " + reason + "."
}

func statIndex(stats []InvariantStageStat) map[int64]InvariantStageStat {
	out := make(map[int64]InvariantStageStat, len(stats))
	for _, stat := range stats {
		out[stat.InvariantID] = stat
	}
	return out
}

func filterBlocking(invariants []ActiveInvariant) []ActiveInvariant {
	return slices.DeleteFunc(slices.Clone(invariants), func(invariant ActiveInvariant) bool {
		return invariant.EnforcementMode != "block"
	})
}

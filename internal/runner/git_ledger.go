package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// GitLedger implements Ledger using a Git repository.
type GitLedger struct {
	repoPath string
}

func NewGitLedger(repoPath string) *GitLedger {
	return &GitLedger{repoPath: repoPath}
}

// Lookup finds the SHA of a successful state transition from the ledger.
func (l *GitLedger) Lookup(ctx context.Context, contract Contract, input SHA) (SHA, bool) {
	ledgerPath := filepath.Join(l.repoPath, ".autodev", "ledger", contract.Name, string(input), "result.sha")
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return "", false
	}
	
	return SHA(strings.TrimSpace(string(data))), true
}

// Commit records a successful state transition by creating a ledger entry.
func (l *GitLedger) Commit(ctx context.Context, contract Contract, input, output SHA, stats map[string]float64) error {
	ledgerPath := filepath.Join(l.repoPath, ".autodev", "ledger", contract.Name, string(input))
	if err := os.MkdirAll(ledgerPath, 0755); err != nil {
		return err
	}
	
	return os.WriteFile(filepath.Join(ledgerPath, "result.sha"), []byte(output), 0644)
}

func (l *GitLedger) Load(ctx context.Context, sha SHA) ([]byte, error) {
    return []byte("mock-data"), nil
}
func (l *GitLedger) Store(ctx context.Context, data []byte) (SHA, error) {
    return "SHA-STORED", nil
}

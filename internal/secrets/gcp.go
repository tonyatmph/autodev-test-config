package secrets

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GCPProvider struct {
	Project string
}

func (p GCPProvider) Resolve(ctx context.Context, name string) (Value, error) {
	if strings.TrimSpace(p.Project) == "" {
		return Value{}, fmt.Errorf("%w: missing GCP project", ErrNotFound)
	}
	cmd := exec.CommandContext(
		ctx,
		"gcloud",
		"secrets",
		"versions",
		"access",
		"latest",
		"--secret", name,
		"--project", p.Project,
		"--quiet",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		text := stderr.String()
		if strings.Contains(text, "NOT_FOUND") || strings.Contains(text, "was not found") {
			return Value{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return Value{}, fmt.Errorf("resolve gcp secret %s: %w", name, err)
	}
	return Value{
		Name:   name,
		Source: "gcp-secret-manager:" + p.Project + "/" + name,
		Value:  strings.TrimSpace(stdout.String()),
	}, nil
}

//go:build darwin

package secrets

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type KeychainProvider struct {
	Service string
}

func (p KeychainProvider) Resolve(ctx context.Context, name string) (Value, error) {
	service := p.Service
	if service == "" {
		service = "autodev"
	}
	account := NormalizeKeychainName(name)
	serviceRef := service + "/" + account
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-a", account, "-s", serviceRef, "-w")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "could not be found") {
			return Value{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return Value{}, fmt.Errorf("resolve keychain secret %s: %w", name, err)
	}
	return Value{
		Name:   name,
		Source: "keychain:" + serviceRef,
		Value:  strings.TrimSpace(stdout.String()),
	}, nil
}

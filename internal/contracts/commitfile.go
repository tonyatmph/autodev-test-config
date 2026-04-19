package contracts

import (
	"fmt"
	"regexp"
	"strings"
)

var gitCommitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

func ValidateCommit(documentName string, data []byte) error {
	commit := strings.TrimSpace(string(data))
	if !gitCommitSHA.MatchString(commit) {
		return fmt.Errorf("%s must contain exactly one 40-character lowercase git commit sha", documentName)
	}
	return nil
}

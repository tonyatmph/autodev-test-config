package llm

import "context"

// Client is a generic interface for LLM operations.
type Client interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

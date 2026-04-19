package llm

import (
	"context"
)

type OpenAIClient struct{}
func NewOpenAIClient(apiKey, model string) *OpenAIClient { return &OpenAIClient{} }
func (c *OpenAIClient) Generate(ctx context.Context, prompt string) (string, error) { return "openai-response", nil }

type OllamaClient struct{}
func NewOllamaClient(url, model string) *OllamaClient { return &OllamaClient{} }
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) { return "ollama-response", nil }

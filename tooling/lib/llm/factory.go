package llm

import (
	"fmt"
	"os"
)

type Provider string
const (
    ProviderOpenAI    Provider = "openai"
    ProviderGemini    Provider = "gemini"
    ProviderOllama    Provider = "ollama"
)

func NewClient(p Provider, model string) (Client, error) {
    switch p {
    case ProviderOpenAI:
        return &OpenAIClient{}, nil
    case ProviderGemini:
        return NewGeminiClient(os.Getenv("GEMINI_API_KEY"), model), nil
    case ProviderOllama:
        return &OllamaClient{}, nil
    default:
        return nil, fmt.Errorf("unsupported provider: %s", p)
    }
}

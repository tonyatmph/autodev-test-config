package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/tooling/lib/llm"
)

type Intent struct {
	Task string `json:"task"`
}

func main() {
	modelName := flag.String("model", "gemini-3.1-flash-lite-preview", "LLM model to use")
	providerName := flag.String("provider", "gemini", "LLM provider (openai, gemini, ollama)")
	flag.Parse()

	contextPath := os.Getenv("AUTODEV_STAGE_CONTEXT")
	resultPath := os.Getenv("AUTODEV_STAGE_RESULT")

	// If no context, it might be a help call or error
	if contextPath == "" || resultPath == "" {
		log.Fatal("AUTODEV_STAGE_CONTEXT and AUTODEV_STAGE_RESULT must be set")
	}

	// 1. Read context/intent
	var intent Intent
	if err := contracts.ReadFile(contextPath, "", &intent); err != nil {
		log.Fatalf("failed to read context: %v", err)
	}

	// 2. Instantiate Client
	client, err := llm.NewClient(llm.Provider(*providerName), *modelName)
	if err != nil {
		log.Fatalf("failed to init llm: %v", err)
	}

	// 3. Perform Cognition
	// We ask the LLM to format the output as JSON for easy parsing.
	prompt := fmt.Sprintf("Execute this task: %s. Return the result as a JSON object: {\"file\": \"filename\", \"content\": \"filecontent\"}", intent.Task)
	response, err := client.Generate(context.Background(), prompt)
	if err != nil {
		log.Fatalf("llm call failed: %v", err)
	}

	// 4. Emit Result
	// The Orchestrator expects a standardized result.json contract.
	result := map[string]any{
		"status":   "succeeded",
		"fitness":  1.0,
		"response": response, // This contains the JSON blob
	}
	if err := contracts.WriteFile(resultPath, contracts.StageResultSchema, result); err != nil {
		log.Fatalf("failed to write result: %v", err)
	}
}

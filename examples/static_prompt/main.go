// Example: Agent with a static system prompt.
//
// This is the simplest setup — the system prompt never changes.
// Uses Agent.Send which handles the full conversation loop.
//
// Usage:
//
//	go run ./examples/static_prompt/
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
)

func main() {
	_ = godotenv.Load()

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant. Do your best to respond to the user accurately and concisely."),
	}

	reply, err := agent.Send(context.Background(), "What is the capital of France?")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)
}

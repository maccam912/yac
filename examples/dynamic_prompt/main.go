// Example: Agent with a dynamic system prompt that re-renders every call.
//
// The system prompt includes the current date/time and day of week.
// TemplatePrompt re-evaluates on every Agent.Send call.
//
// Usage:
//
//	go run ./examples/dynamic_prompt/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
)

var systemTemplate = template.Must(template.New("system").Parse(
	`You are a friendly, helpful assistant.
The current date and time is: {{.DateTime}}
Today is: {{.DayOfWeek}}

Always mention what day of the week it is somewhere in your response.`))

func main() {
	_ = godotenv.Load()

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.TemplatePrompt(systemTemplate, func() any {
			now := time.Now()
			return map[string]string{
				"DateTime":  now.Format("2006-01-02 15:04:05"),
				"DayOfWeek": now.Weekday().String(),
			}
		}),
	}

	fmt.Println("=== System Prompt ===")
	fmt.Println(agent.SystemPrompt())
	fmt.Println("=====================")
	fmt.Println()

	reply, err := agent.Send(context.Background(), "What's a good thing to do today?")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)
}

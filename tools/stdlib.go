package tools

import (
	"github.com/maccam912/yac"
)

// All returns all standard tools from the yac tools library.
//
// This includes:
//   - Calculator: math expression evaluator
//   - WebRequest: HTTP client for fetching web resources
//   - SearXNG: web search (only if SEARXNG_URL env var is set)
//
// Tools with conditional inclusion (like SearXNG) are included in the
// slice but will be filtered out by yac.FilterTools() if their conditions
// aren't met.
//
// Note: Delegate is NOT included since it requires configuration (adapter,
// tools for subagents, etc.). Use Delegate() separately if needed.
//
// Example:
//
//	allTools := tools.All()
//	agent := yac.Agent{
//	    Tools: yac.FilterTools(allTools),
//	}
func All() []*yac.Tool {
	return []*yac.Tool{
		Calculator(),
		WebRequest(),
		SearXNG(),
		Bash(),
	}
}

// AllWithDelegate returns all standard tools plus a Delegate tool
// configured with the given settings.
//
// The Delegate tool allows the agent to spawn subagents for parallel
// task execution. Subagents will have access to the same tools minus
// Delegate (to prevent infinite recursion at the final depth level).
//
// Example:
//
//	adapter := &yac.OpenAIAdapter{...}
//	allTools := tools.AllWithDelegate(adapter, 2)
//	agent := yac.Agent{
//	    Tools: yac.FilterTools(allTools),
//	}
func AllWithDelegate(adapter yac.Adapter, maxDepth int) []*yac.Tool {
	baseTools := All()
	delegateTool := Delegate(DelegateConfig{
		Adapter:  adapter,
		Tools:    baseTools,
		MaxDepth: maxDepth,
	})
	return append(baseTools, delegateTool)
}

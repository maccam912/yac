// Package tools provides a standard library of ready-to-use tools
// for yac agents.
//
// Each tool is exposed as a constructor function that returns a
// configured *yac.Tool, ready to be added to an agent:
//
//	agent := yac.Agent{
//	    Tools: []*yac.Tool{
//	        tools.Calculator(),
//	    },
//	}
//
// All tools follow the same pattern: they take no configuration
// (or optional configuration via functional options) and return
// a *yac.Tool with Name, Description, Parameters, and Execute
// already wired up.
package tools

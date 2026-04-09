package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MURDERRA/MultiAI-workflow/internal/config"
	"github.com/MURDERRA/MultiAI-workflow/internal/file"
	"github.com/MURDERRA/MultiAI-workflow/internal/providers"
)

const clarifySystemPrompt = `You are an expert prompt engineer. Your job is to help clarify a user's task so it can be executed perfectly by an AI model.

Given a task, generate 2-5 targeted clarifying questions that would help you only if needed and you need any external information:
1. Understand the exact requirements
2. Identify constraints and preferences
3. Determine the expected output format

Respond ONLY with a JSON object in this exact format (no markdown, no explanation):
{
  "questions": [
    "question 1",
    "question 2",
    "question 3"
  ]
}

If you don't need any external info, then just respond like this
{
  "questions": [
    "No questions"
  ]
}
`

const finalizeSystemPrompt = `You are an expert task router and prompt engineer. Given a user task and answers to clarifying questions, you must:

1. BUILD A COMPLETE PROMPT:
   - Incorporate all context from clarifying question answers
   - Make it precise, actionable, and self-contained
   - Preserve technical details and constraints mentioned by the user

2. DETERMINE TASK TYPE using these rules:
   - "text": writing, analysis, explanation, research, documentation, translation, summarization, brainstorming
   - "code": routine/repetitive code, tests, boilerplate, scripts, simple utilities, debugging, refactoring small functions
   - "quality_code": complex algorithms, performance-critical code, code requiring deep reasoning, intricate bug fixes, optimization
   - "architecture": system design, architecture decisions, project structure, technology stack choices, scalability planning
   - "big_text": long-form content (articles, books, extensive reports), multi-section documentation, comprehensive research analysis
   
3. USER ENVIRONMENT CONTEXT (include when relevant):
   - OS: Arch Linux + Hyprland (dots from illogical impulse)
   - Shell: fish (NOT bash — use fish syntax for scripts/aliases)
   - Editor: nvim for quick edits, VSCode for development
   - Python: Arch doesn't support global pip installs, only venv (see PEP 668)

Respond ONLY with a JSON object in this exact format (no markdown, no explanation, no code blocks):
{
  "prompt": "the complete refined prompt with all context incorporated",
  "route": "text|code|quality_code|architecture|big_text",
  "reason": "one sentence explaining why this route was chosen"
}`

type ClarifyResult struct {
	Questions []string `json:"questions"`
}

type FinalizeResult struct {
	Prompt string `json:"prompt"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

type Router struct {
	cfg      *config.Config
	provider providers.Provider
}

func New(cfg *config.Config) (*Router, error) {
	provCfg, err := cfg.FindProvider(cfg.Router.ClarifyProvider)
	if err != nil {
		return nil, fmt.Errorf("clarify provider: %w", err)
	}
	prov, err := providers.New(provCfg)
	if err != nil {
		return nil, err
	}
	return &Router{cfg: cfg, provider: prov}, nil
}

// Clarify sends the task to the clarify provider and returns questions.
func (r *Router) Clarify(ctx context.Context, task string) ([]string, error) {
	msgs := []providers.Message{
		{Role: "user", Content: fmt.Sprintf("Task: %s", task)},
	}

	resp, err := r.provider.Complete(ctx, msgs, clarifySystemPrompt)
	if err != nil {
		return nil, fmt.Errorf("clarify: %w", err)
	}

	content := cleanJSON(resp.Content)
	var result ClarifyResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("clarify parse error: %w\nraw: %s", err, resp.Content)
	}

	return result.Questions, nil
}

// Finalize builds the final prompt and determines routing.
func (r *Router) Finalize(ctx context.Context, af *file.AIFile) (*FinalizeResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Original task: %s\n\n", af.Task))
	sb.WriteString("Clarifying Q&A:\n")
	for i, qa := range af.QAs {
		sb.WriteString(fmt.Sprintf("Q%d: %s\nA%d: %s\n", i+1, qa.Question, i+1, qa.Answer))
	}

	msgs := []providers.Message{
		{Role: "user", Content: sb.String()},
	}

	resp, err := r.provider.Complete(ctx, msgs, finalizeSystemPrompt)
	if err != nil {
		return nil, fmt.Errorf("finalize: %w", err)
	}

	content := cleanJSON(resp.Content)
	var result FinalizeResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("finalize parse error: %w\nraw: %s", err, resp.Content)
	}

	// Validate route
	switch result.Route {
	case "text", "code", "quality_code":
	default:
		return nil, fmt.Errorf("unknown route %q from clarify provider", result.Route)
	}

	return &result, nil
}

// ResolveProvider maps a route to a provider name from config.
func (r *Router) ResolveProvider(route string) (string, error) {
	name, ok := r.cfg.Router.Routes[route]
	if !ok {
		return "", fmt.Errorf("no route configured for %q", route)
	}
	return name, nil
}

// cleanJSON strips markdown code fences if the model wrapped JSON in them.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// strip first line (```json or ```)
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		}
		// strip trailing ```
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	return strings.TrimSpace(s)
}

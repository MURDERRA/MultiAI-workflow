package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MURDERRA/MultiAI-workflow/internal/config"
	"github.com/MURDERRA/MultiAI-workflow/internal/workflow"
)

const version = "0.1.0"

const helpText = `aiflow — multi-AI prompt workflow tool

Usage:
  aiflow init                     Write default config to ~/.config/aiflow/config.json
  aiflow new <task>               Clarify task, create <slug>.ai, open editor
  aiflow new <task> --no-editor   Create file without opening editor
  aiflow run <file.ai>            Finalize prompt, dispatch to provider, save output
  aiflow run <file.ai> --no-stream  Disable streaming (wait for full response)
  aiflow chat <file.ai> <message> Continue conversation in existing .ai file
  aiflow show <file.ai>           Print parsed content of .ai file
  aiflow version                  Print version

Examples:
  aiflow new "build a REST API for user auth in Go"
  aiflow run my-api.ai
  aiflow chat my-api.ai "now add refresh token support"

Config: ~/.config/aiflow/config.json
  Set OPENROUTER_API_KEY and ANTHROPIC_API_KEY in your environment,
  or put the values directly in config.json.
`

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Print(helpText)
		os.Exit(0)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "--help", "-h":
		fmt.Print(helpText)

	case "version", "--version", "-v":
		fmt.Printf("aiflow %s\n", version)

	case "init":
		runInit()

	case "new":
		runNew(rest)

	case "run":
		runRun(rest)

	case "chat":
		runChat(rest)

	case "show":
		runShow(rest)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Print(helpText)
		os.Exit(1)
	}
}

func runInit() {
	path := config.ConfigPath()
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Config already exists at %s\n", path)
		fmt.Printf("Delete it first if you want to reset.\n")
		return
	}
	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		fatal("save config: %v", err)
	}
	fmt.Printf("✅ Config written to %s\n\n", path)
	fmt.Printf("Next steps:\n")
	fmt.Printf("  1. Set your API keys:\n")
	fmt.Printf("     export OPENROUTER_API_KEY=sk-or-...\n")
	fmt.Printf("     export ANTHROPIC_API_KEY=sk-ant-...\n")
	fmt.Printf("  2. Or edit the config and put keys directly.\n")
	fmt.Printf("  3. Try: aiflow new \"your task here\"\n")
}

func runNew(args []string) {
	if len(args) == 0 {
		fatal("Usage: aiflow new <task description>")
	}

	skipEditor := false
	var taskParts []string
	for _, a := range args {
		if a == "--no-editor" {
			skipEditor = true
		} else {
			taskParts = append(taskParts, a)
		}
	}
	task := strings.Join(taskParts, " ")

	cfg := mustLoadConfig()
	wf, err := workflow.New(cfg)
	if err != nil {
		fatal("init workflow: %v", err)
	}

	ctx := context.Background()
	if err := wf.New(ctx, task, skipEditor); err != nil {
		fatal("%v", err)
	}
}

func runRun(args []string) {
	if len(args) == 0 {
		fatal("Usage: aiflow run <file.ai>")
	}

	path := ""
	stream := true
	for _, a := range args {
		if a == "--no-stream" {
			stream = false
		} else if !strings.HasPrefix(a, "--") {
			path = a
		}
	}
	if path == "" {
		fatal("Usage: aiflow run <file.ai>")
	}

	cfg := mustLoadConfig()
	wf, err := workflow.New(cfg)
	if err != nil {
		fatal("init workflow: %v", err)
	}

	ctx := context.Background()
	if err := wf.Run(ctx, path, stream); err != nil {
		fatal("%v", err)
	}
}

func runChat(args []string) {
	if len(args) < 2 {
		fatal("Usage: aiflow chat <file.ai> <message>")
	}

	path := args[0]
	stream := true
	var msgParts []string

	for _, a := range args[1:] {
		if a == "--no-stream" {
			stream = false
		} else {
			msgParts = append(msgParts, a)
		}
	}
	message := strings.Join(msgParts, " ")

	if message == "" {
		fatal("message cannot be empty")
	}

	cfg := mustLoadConfig()
	wf, err := workflow.New(cfg)
	if err != nil {
		fatal("init workflow: %v", err)
	}

	ctx := context.Background()
	if err := wf.Chat(ctx, path, message, stream); err != nil {
		fatal("%v", err)
	}
}

func runShow(args []string) {
	if len(args) == 0 {
		fatal("Usage: aiflow show <file.ai>")
	}

	// Just pretty-print the raw file with section highlights
	data, err := os.ReadFile(args[0])
	if err != nil {
		fatal("read file: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			// Section header — bold/colored via ANSI
			fmt.Printf("\033[1;36m%s\033[0m\n", line)
		} else if strings.HasPrefix(trimmed, "<<") {
			fmt.Printf("\033[1;33m%s\033[0m\n", line)
		} else if strings.HasPrefix(trimmed, "Q") && len(trimmed) > 2 && trimmed[1] >= '0' && trimmed[1] <= '9' {
			fmt.Printf("\033[1;32m%s\033[0m\n", line)
		} else if strings.HasPrefix(trimmed, "A") && len(trimmed) > 2 && trimmed[1] >= '0' && trimmed[1] <= '9' {
			fmt.Printf("\033[0;32m%s\033[0m\n", line)
		} else {
			fmt.Println(line)
		}
	}
}

func mustLoadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		fatal("%v", err)
	}
	return cfg
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}

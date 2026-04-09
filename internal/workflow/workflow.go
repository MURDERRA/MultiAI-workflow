package workflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MURDERRA/MultiAI-workflow/internal/config"
	"github.com/MURDERRA/MultiAI-workflow/internal/file"
	"github.com/MURDERRA/MultiAI-workflow/internal/providers"
	"github.com/MURDERRA/MultiAI-workflow/internal/router"
)

type Workflow struct {
	cfg    *config.Config
	router *router.Router
}

func New(cfg *config.Config) (*Workflow, error) {
	r, err := router.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Workflow{cfg: cfg, router: r}, nil
}

// New creates a .ai file, fills it with clarifying questions, opens editor.
func (w *Workflow) New(ctx context.Context, taskOrPath string, skipEditor bool) error {
	// Determine path and task
	var path, task string
	if strings.HasSuffix(taskOrPath, ".ai") {
		path = taskOrPath
		task = ""
	} else {
		task = taskOrPath
		// Derive filename from first few words
		words := strings.Fields(task)
		if len(words) > 4 {
			words = words[:4]
		}
		slug := strings.ToLower(strings.Join(words, "-"))
		slug = sanitizeFilename(slug)
		path = slug + ".ai"
	}

	fmt.Printf("⏳ Generating clarifying questions via %s...\n", w.cfg.Router.ClarifyProvider)

	questions, err := w.router.Clarify(ctx, task)
	if err != nil {
		return fmt.Errorf("clarify failed: %w", err)
	}

	af, err := file.New(path, task)
	if err != nil {
		return err
	}

	// Fill in questions with empty answers
	af.QAs = make([]file.QA, len(questions))
	for i, q := range questions {
		af.QAs[i] = file.QA{Question: q, Answer: ""}
	}

	if err := af.Write(); err != nil {
		return err
	}

	fmt.Printf("✅ Created %s with %d questions\n", path, len(questions))
	fmt.Printf("   Fill in the answers (A1:, A2:, ...) then run:\n")
	fmt.Printf("   aiflow run %s\n\n", path)

	if !skipEditor {
		return openEditor(w.cfg.GetEditor(), path)
	}
	return nil
}

// Run reads a filled .ai file, builds final prompt, dispatches to provider.
func (w *Workflow) Run(ctx context.Context, path string, stream bool) error {
	af, err := file.Load(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}

	// If no prompt yet, need to finalize first
	if af.Prompt == "" {
		if !af.AnswersComplete() {
			return fmt.Errorf("not all answers filled in %s\nOpen the file and fill in A1:, A2:, etc.", path)
		}

		fmt.Printf("⏳ Building final prompt via %s...\n", w.cfg.Router.ClarifyProvider)
		result, err := w.router.Finalize(ctx, af)
		if err != nil {
			return fmt.Errorf("finalize failed: %w", err)
		}

		af.Prompt = result.Prompt
		af.Route = result.Route
		provName, err := w.router.ResolveProvider(result.Route)
		if err != nil {
			return err
		}
		af.Provider = provName
		af.Reason = result.Reason
		if err := af.Write(); err != nil {
			return err
		}

		fmt.Printf("✅ Route: %s → %s (%s)\n", result.Route, provName, result.Reason)
	} else {
		fmt.Printf("✅ Using existing prompt → %s (%s)\n", af.Provider, af.Route)
	}

	// Get provider
	provCfg, err := w.cfg.FindProvider(af.Provider)
	if err != nil {
		return err
	}
	prov, err := providers.New(provCfg)
	if err != nil {
		return err
	}

	// Build messages (include history if present)
	messages := historyToMessages(af.History)
	messages = append(messages, providers.Message{Role: "user", Content: af.Prompt})

	fmt.Printf("⏳ Sending to %s...\n\n", af.Provider)

	var responseText string

	if stream {
		ch := make(chan providers.StreamChunk, 32)
		go prov.Stream(ctx, messages, "", ch)
		var sb strings.Builder
		for chunk := range ch {
			if chunk.Err != nil {
				return fmt.Errorf("stream error: %w", chunk.Err)
			}
			if chunk.Delta != "" {
				fmt.Print(chunk.Delta)
				sb.WriteString(chunk.Delta)
			}
		}
		fmt.Println()
		responseText = sb.String()
	} else {
		resp, err := prov.Complete(ctx, messages, "")
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		responseText = resp.Content
		fmt.Println(responseText)
	}

	// Save to output file
	outPath := af.OutputPath()
	header := fmt.Sprintf("# Task: %s\n\n**Provider:** %s  \n**Route:** %s  \n**Reason:** %s\n\n---\n\n",
		af.Task, af.Provider, af.Route, af.Reason)
	if err := os.WriteFile(outPath, []byte(header+responseText), 0644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	// Update history in .ai file
	_ = af.AddHistory("user", af.Prompt)
	_ = af.AddHistory("assistant", responseText)

	fmt.Printf("\n📄 Saved to %s\n", outPath)
	return nil
}

// Chat continues a conversation based on an existing .ai file.
func (w *Workflow) Chat(ctx context.Context, path, userMessage string, stream bool) error {
	af, err := file.Load(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}

	if af.Provider == "" {
		return fmt.Errorf("no provider set in %s — run 'aiflow run %s' first", path, path)
	}

	provCfg, err := w.cfg.FindProvider(af.Provider)
	if err != nil {
		return err
	}
	prov, err := providers.New(provCfg)
	if err != nil {
		return err
	}

	messages := historyToMessages(af.History)
	messages = append(messages, providers.Message{Role: "user", Content: userMessage})

	fmt.Printf("⏳ [%s] ...\n\n", af.Provider)

	var responseText string

	if stream {
		ch := make(chan providers.StreamChunk, 32)
		go prov.Stream(ctx, messages, "", ch)
		var sb strings.Builder
		for chunk := range ch {
			if chunk.Err != nil {
				return fmt.Errorf("stream error: %w", chunk.Err)
			}
			if chunk.Delta != "" {
				fmt.Print(chunk.Delta)
				sb.WriteString(chunk.Delta)
			}
		}
		fmt.Println()
		responseText = sb.String()
	} else {
		resp, err := prov.Complete(ctx, messages, "")
		if err != nil {
			return err
		}
		responseText = resp.Content
		fmt.Println(responseText)
	}

	// Append to history
	_ = af.AddHistory("user", userMessage)
	_ = af.AddHistory("assistant", responseText)

	// Append to output file
	outPath := af.OutputPath()
	appendText := fmt.Sprintf("\n\n---\n\n**You:** %s\n\n%s", userMessage, responseText)
	f, err := os.OpenFile(outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(appendText)
		f.Close()
	}

	fmt.Printf("\n💬 History updated in %s\n", path)
	return nil
}

// SaveHistory copies an .ai file to the history directory.
func (w *Workflow) SaveHistory(af *file.AIFile) error {
	dir := w.cfg.HistoryDir
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := file.HistoryPath(dir, af.Task)
	data, err := os.ReadFile(af.Path)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}

func openEditor(editor, path string) error {
	// Get absolute path
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cmd := exec.Command(editor, abs)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func historyToMessages(history []file.Message) []providers.Message {
	msgs := make([]providers.Message, len(history))
	for i, h := range history {
		msgs[i] = providers.Message{Role: h.Role, Content: h.Content}
	}
	return msgs
}

func sanitizeFilename(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-' || r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	result := sb.String()
	// Collapse multiple dashes
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

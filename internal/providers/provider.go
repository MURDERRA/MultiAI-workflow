package providers

import (
	"context"
	"fmt"

	"github.com/MURDERRA/MultiAI-workflow/internal/config"
)

// Message is a chat message.
type Message struct {
	Role    string
	Content string
}

// Response from a provider.
type Response struct {
	Content string
	// For streaming, content is accumulated here too.
}

// StreamChunk is a piece of a streaming response.
type StreamChunk struct {
	Delta string
	Done  bool
	Err   error
}

// Provider is the interface every backend must implement.
type Provider interface {
	// Complete sends messages and returns a full response.
	Complete(ctx context.Context, messages []Message, system string) (*Response, error)

	// Stream sends messages and streams chunks to the channel.
	// The channel is closed when done or on error (last chunk has Err set).
	Stream(ctx context.Context, messages []Message, system string, ch chan<- StreamChunk)

	Name() string
}

// New creates the right provider based on config type.
func New(cfg *config.ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case "openrouter", "openai_compat":
		return NewOpenAICompat(cfg), nil
	case "anthropic":
		return NewAnthropic(cfg), nil
	case "ollama":
		ollamaCfg := *cfg
		if ollamaCfg.BaseURL == "" {
			ollamaCfg.BaseURL = "http://localhost:11434"
		}
		return NewOpenAICompat(&ollamaCfg), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", cfg.Type)
	}
}

package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MURDERRA/MultiAI-workflow/internal/config"
)

const anthropicVersion = "2023-06-01"

type Anthropic struct {
	cfg    *config.ProviderConfig
	client *http.Client
}

func NewAnthropic(cfg *config.ProviderConfig) *Anthropic {
	return &Anthropic{
		cfg:    cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *Anthropic) Name() string { return p.cfg.Name }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// SSE event types from Anthropic streaming API
type anthropicSSE struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Anthropic) buildMessages(messages []Message) []anthropicMessage {
	var msgs []anthropicMessage
	for _, m := range messages {
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}
	return msgs
}

func (p *Anthropic) baseURL() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	return "https://api.anthropic.com"
}

func (p *Anthropic) Complete(ctx context.Context, messages []Message, system string) (*Response, error) {
	req := anthropicRequest{
		Model:     p.cfg.Model,
		MaxTokens: 8192,
		Messages:  p.buildMessages(messages),
		Stream:    false,
	}
	if system != "" {
		req.System = system
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL()+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}

	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return &Response{Content: text.String()}, nil
}

func (p *Anthropic) Stream(ctx context.Context, messages []Message, system string, ch chan<- StreamChunk) {
	defer close(ch)

	req := anthropicRequest{
		Model:     p.cfg.Model,
		MaxTokens: 8192,
		Messages:  p.buildMessages(messages),
		Stream:    true,
	}
	if system != "" {
		req.System = system
	}

	body, err := json.Marshal(req)
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL()+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		ch <- StreamChunk{Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b)), Done: true}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event anthropicSSE
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				ch <- StreamChunk{Delta: event.Delta.Text}
			}
		case "message_stop":
			ch <- StreamChunk{Done: true}
			return
		case "error":
			if event.Error != nil {
				ch <- StreamChunk{Err: fmt.Errorf("stream error: %s", event.Error.Message), Done: true}
				return
			}
		}
	}
}

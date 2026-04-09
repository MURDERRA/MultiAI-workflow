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

type OpenAICompat struct {
	cfg    *config.ProviderConfig
	client *http.Client
}

func NewOpenAICompat(cfg *config.ProviderConfig) *OpenAICompat {
	return &OpenAICompat{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAICompat) Name() string { return p.cfg.Name }

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *OpenAICompat) buildMessages(messages []Message, system string) []openAIMessage {
	var msgs []openAIMessage
	if system != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: system})
	}
	for _, m := range messages {
		msgs = append(msgs, openAIMessage{Role: m.Role, Content: m.Content})
	}
	return msgs
}

func (p *OpenAICompat) Complete(ctx context.Context, messages []Message, system string) (*Response, error) {
	req := openAIRequest{
		Model:    p.cfg.Model,
		Messages: p.buildMessages(messages, system),
		Stream:   false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

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
		return nil, fmt.Errorf("provider %s HTTP %d: %s", p.cfg.Name, resp.StatusCode, string(respBody))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("provider %s error: %s", p.cfg.Name, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("provider %s: no choices in response", p.cfg.Name)
	}

	return &Response{Content: parsed.Choices[0].Message.Content}, nil
}

func (p *OpenAICompat) Stream(ctx context.Context, messages []Message, system string, ch chan<- StreamChunk) {
	defer close(ch)

	req := openAIRequest{
		Model:    p.cfg.Model,
		Messages: p.buildMessages(messages, system),
		Stream:   true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}

	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

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
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true}
			return
		}
		var parsed openAIResponse
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			continue
		}
		if len(parsed.Choices) > 0 {
			delta := parsed.Choices[0].Delta.Content
			if delta != "" {
				ch <- StreamChunk{Delta: delta}
			}
			if parsed.Choices[0].FinishReason == "stop" {
				ch <- StreamChunk{Done: true}
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Err: err, Done: true}
	}
}

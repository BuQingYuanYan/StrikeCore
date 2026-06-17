package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// openAIMessage 映射 OpenAI /chat/completions 请求中的消息体。
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest 映射 OpenAI /chat/completions 请求体。
type openAIChatRequest struct {
	Model    string           `json:"model"`
	Messages []openAIMessage  `json:"messages"`
	Stream   bool             `json:"stream,omitempty"`
}

// openAIChoice 映射 OpenAI 响应中的一个 choice。
type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

// openAIChatResponse 映射 OpenAI /chat/completions 响应体。
type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
}

// OpenAIConfig 是 OpenAI 兼容 API 的连接参数。
type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type openAIProvider struct {
	config OpenAIConfig
	client *http.Client
}

// NewOpenAIProvider 创建 OpenAI 兼容 API 的 LLM Provider。
// BaseURL 示例：https://api.minimax.chat/v1
func NewOpenAIProvider(cfg OpenAIConfig) Provider {
	return &openAIProvider{
		config: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    2,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (p *openAIProvider) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (string, error) {
	model := p.config.Model
	if opts != nil && opts.Model != "" {
		model = opts.Model
	}

	apiMsgs := make([]openAIMessage, 0, len(messages)+1)
	if opts != nil && opts.SystemPrompt != "" {
		apiMsgs = append(apiMsgs, openAIMessage{Role: "system", Content: opts.SystemPrompt})
	}
	for _, m := range messages {
		apiMsgs = append(apiMsgs, openAIMessage{Role: m.Role, Content: m.Content})
	}

	body := openAIChatRequest{
		Model:    model,
		Messages: apiMsgs,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: API error (HTTP %d): %s", resp.StatusCode, string(respRaw))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respRaw, &chatResp); err != nil {
		return "", fmt.Errorf("llm: parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm: empty response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

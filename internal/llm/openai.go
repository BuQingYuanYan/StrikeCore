package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// openAIMessage 映射 OpenAI /chat/completions 请求中的消息体。
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest 映射 OpenAI /chat/completions 请求体。
type openAIChatRequest struct {
	Model         string               `json:"model"`
	Messages      []openAIMessage      `json:"messages"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

// openAIStreamOptions 控制流式响应中的额外数据，例如 usage。
type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIChoice 映射 OpenAI 响应中的一个 choice。
type openAIChoice struct {
	Message struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
	} `json:"message"`
}

// openAIChatResponse 映射 OpenAI /chat/completions 响应体。
type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
}

// openAIStreamDelta 映射 SSE 流式响应中的 delta 块。
type openAIStreamDelta struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// openAIStreamChoice 映射 SSE 流式响应中的一个 choice。
type openAIStreamChoice struct {
	Delta openAIStreamDelta `json:"delta"`
	Index int               `json:"index"`
}

// openAIStreamChunk 映射 SSE 流式响应中的一行 data: JSON。
type openAIStreamChunk struct {
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage         `json:"usage,omitempty"`
}

// openAIUsage 由 API 在流末附带返回。
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
// BaseURL 示例：https://api.example.com/v1
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

func (p *openAIProvider) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (Message, error) {
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
		return Message{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Message{}, fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("llm: API error (HTTP %d): %s", resp.StatusCode, string(respRaw))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respRaw, &chatResp); err != nil {
		return Message{}, fmt.Errorf("llm: parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return Message{}, fmt.Errorf("llm: empty response")
	}

	msg := chatResp.Choices[0].Message
	return Message{Role: msg.Role, Content: msg.Content, ReasoningContent: msg.ReasoningContent}, nil
}

func (p *openAIProvider) ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan Message, error) {
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
		Stream:   true,
		StreamOptions: &openAIStreamOptions{
			IncludeUsage: true,
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("llm: API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	ch := make(chan Message, 8)
	go func() {
		var closeOnce sync.Once
		closeBody := func() { resp.Body.Close() }
		defer closeOnce.Do(closeBody)
		defer close(ch)

		// context 取消时关闭响应体，使阻塞的 Read 立即返回，避免 goroutine 泄漏
		go func() {
			<-ctx.Done()
			closeOnce.Do(closeBody)
		}()

		br := NewSSEReader(resp.Body)
		for {
			data, ok := br.Next()
			if !ok {
				return
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal(data, &chunk); err != nil {
				continue
			}
			// 解析 usage（可能在 choices 为空的最后一行，或随末条 choice 一起返回）
			var usage *Usage
			if chunk.Usage != nil {
				usage = &Usage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
			}
			if len(chunk.Choices) == 0 {
				if usage != nil {
					select {
					case ch <- Message{Usage: usage}:
					case <-ctx.Done():
					}
				}
				continue
			}
			delta := chunk.Choices[0].Delta
			if delta.Content == "" && delta.ReasoningContent == "" {
				if usage != nil {
					select {
					case ch <- Message{Usage: usage}:
					case <-ctx.Done():
					}
				}
				continue
			}
			msg := Message{Role: "assistant", Content: delta.Content, Usage: usage}
			if delta.ReasoningContent != "" {
				msg.ReasoningContent = delta.ReasoningContent
			}
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// SSEReader 逐行读取 SSE 流，提取 data: 前缀后的 JSON 内容。
type SSEReader struct {
	rd  io.Reader
	buf []byte
}

// NewSSEReader 创建一个 SSE 读取器。
func NewSSEReader(rd io.Reader) *SSEReader {
	return &SSEReader{rd: rd}
}

// Next 返回下一条 data: 行的 JSON 内容。ok=false 表示流结束。
func (s *SSEReader) Next() ([]byte, bool) {
	for {
		line, err := s.readLine()
		if err != nil {
			return nil, false
		}
		if len(line) == 0 {
			continue
		}
		if len(line) > 6 && string(line[:6]) == "data: " {
			content := line[6:]
			if string(content) == "[DONE]" {
				return nil, false
			}
			return content, true
		}
	}
}

// readLine 读取一行（以 \n 或 \r\n 结尾），去掉结尾换行符。
func (s *SSEReader) readLine() ([]byte, error) {
	for {
		for i, b := range s.buf {
			if b == '\n' {
				line := s.buf[:i]
				if i > 0 && line[i-1] == '\r' {
					line = line[:i-1]
				}
				s.buf = s.buf[i+1:]
				return line, nil
			}
		}
		tmp := make([]byte, 4096)
		n, err := s.rd.Read(tmp)
		if n > 0 {
			s.buf = append(s.buf, tmp[:n]...)
		}
		if err != nil {
			if len(s.buf) > 0 {
				line := s.buf
				s.buf = nil
				return line, nil
			}
			return nil, err
		}
	}
}

// Package llm 提供与大模型 API 交互的通用接口及 OpenAI 兼容实现。
package llm

import "context"

// Usage 表示一次 API 调用的 token 消耗统计。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Message 表示一条对话消息或流式输出块。
// 流式场景下 Usage 仅在最后一个消息块中有值，其余为 nil。
type Message struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Usage            *Usage `json:"usage,omitempty"`
}

// Provider 是 LLM 后端的抽象接口，支持切换不同模型供应商。
type Provider interface {
	Chat(ctx context.Context, messages []Message, opts *ChatOptions) (Message, error)
	// ChatStream 以流式方式调用 LLM，每次有增量时通过 channel 发送 Message（只填充 Content），
	// channel 在流结束后关闭。
	ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan Message, error)
}

// ChatOptions 控制每次 Chat 调用的参数。
type ChatOptions struct {
	SystemPrompt string
	Model        string
}

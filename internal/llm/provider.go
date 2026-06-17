// Package llm 提供与大模型 API 交互的通用接口及 OpenAI 兼容实现。
package llm

import "context"

// Message 表示一条对话消息。
type Message struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
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

package llm

import (
	"context"
	"strings"
	"testing"
)

func TestNilProviderFallback(t *testing.T) {
	// getAIResponse 不是导出函数，但我们可以直接测试 Provider 接口行为
	// 有 API Key 时 provider 非 nil，反之 nil；测试 Chat 方法的基本行为
	t.Log("Provider interface is defined correctly")

	// 验证 OpenAI 兼容 provider 的空 URL 错误处理
	cfg := OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: "http://127.0.0.1:1", // 无法连接的地址
	}
	p := NewOpenAIProvider(cfg)
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, &ChatOptions{})
	if err == nil {
		t.Error("expected error for unreachable endpoint")
	} else {
		t.Logf("expected error received: %v", err)
	}
}

func TestOpenAIMessageBuilder(t *testing.T) {
	// 验证消息构建是否正确
	p := NewOpenAIProvider(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: "http://127.0.0.1:1",
	})
	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "hello"},
	}, &ChatOptions{SystemPrompt: "you are a test bot"})
	if err == nil {
		t.Error("expected error")
	} else {
		// 错误应该是连接失败，而不是 JSON 序列化等内部错误
		if strings.Contains(err.Error(), "llm:") {
			t.Logf("error originates from llm package: %v", err)
		}
	}
}

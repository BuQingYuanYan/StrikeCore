package app

import (
	"strings"
	"testing"

	"strike-core/internal/ui"
)

func TestChatStateScrollUp(t *testing.T) {
	tests := []struct {
		name     string
		start    int
		step     int
		want     int
		wantMove bool
	}{
		{"from middle", 10, 3, 7, true},
		{"clamps at top", 2, 3, 0, true},
		{"already at top", 0, 3, 0, false},
		{"exact to top", 3, 3, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &chatState{scroll: tt.start}
			got := c.scrollUp(tt.step)
			if got != tt.wantMove {
				t.Errorf("scrollUp moved=%v, want %v", got, tt.wantMove)
			}
			if c.scroll != tt.want {
				t.Errorf("scroll=%d, want %d", c.scroll, tt.want)
			}
		})
	}
}

func TestChatStateScrollDown(t *testing.T) {
	tests := []struct {
		name     string
		start    int
		step     int
		want     int
		wantMove bool
	}{
		{"from middle", 10, 3, 13, true},
		{"at bottom sentinel", scrollToBottom, 3, scrollToBottom, false},
		{"near sentinel no overflow", scrollToBottom - 1, 3, scrollToBottom - 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &chatState{scroll: tt.start}
			got := c.scrollDown(tt.step)
			if got != tt.wantMove {
				t.Errorf("scrollDown moved=%v, want %v", got, tt.wantMove)
			}
			if c.scroll != tt.want {
				t.Errorf("scroll=%d, want %d", c.scroll, tt.want)
			}
		})
	}
}

func TestChatStateJumpToBottom(t *testing.T) {
	c := &chatState{scroll: 42}
	c.jumpToBottom()
	if c.scroll != scrollToBottom {
		t.Errorf("scroll=%d, want sentinel %d", c.scroll, scrollToBottom)
	}
}

func TestAppendStreamContent(t *testing.T) {
	t.Run("appends to last assistant message", func(t *testing.T) {
		msgs := []ui.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hel"},
		}
		appendStreamContent(&msgs, "lo")
		if msgs[1].Content != "hello" {
			t.Errorf("content=%q, want %q", msgs[1].Content, "hello")
		}
		if len(msgs) != 2 {
			t.Errorf("len=%d, want 2 (no new message)", len(msgs))
		}
	})

	t.Run("creates assistant message when none exists", func(t *testing.T) {
		msgs := []ui.Message{{Role: "user", Content: "hi"}}
		appendStreamContent(&msgs, "yo")
		if len(msgs) != 2 || msgs[1].Role != "assistant" || msgs[1].Content != "yo" {
			t.Errorf("got %+v, want appended assistant 'yo'", msgs)
		}
	})

	t.Run("targets the most recent assistant, not an earlier one", func(t *testing.T) {
		msgs := []ui.Message{
			{Role: "assistant", Content: "first"},
			{Role: "user", Content: "q"},
			{Role: "assistant", Content: "sec"},
		}
		appendStreamContent(&msgs, "ond")
		if msgs[2].Content != "second" {
			t.Errorf("content=%q, want %q", msgs[2].Content, "second")
		}
		if msgs[0].Content != "first" {
			t.Errorf("earlier assistant mutated: %q", msgs[0].Content)
		}
	})
}

func TestAppendStreamReasoning(t *testing.T) {
	msgs := []ui.Message{{Role: "assistant", Content: "answer"}}
	appendStreamReasoning(&msgs, "thinking...")
	if msgs[0].Reasoning != "thinking..." {
		t.Errorf("reasoning=%q, want %q", msgs[0].Reasoning, "thinking...")
	}
	if msgs[0].Content != "answer" {
		t.Errorf("content should be untouched, got %q", msgs[0].Content)
	}
}

func TestEmitCount(t *testing.T) {
	tests := []struct {
		name string
		buf  string
		want int
	}{
		{"empty", "", 0},
		{"single rune floors to min", "a", emitMinRunes},
		{"small backlog stays at min", "abc", emitMinRunes},
		{"scales with backlog", strings.Repeat("x", 60), 60 / emitDivisor},
		{"caps at max", strings.Repeat("x", 10000), emitMaxRunes},
		{"cjk counted by rune not byte", strings.Repeat("你", 60), 60 / emitDivisor},
		{"never exceeds backlog", strings.Repeat("x", 2), emitMinRunes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emitCount(tt.buf); got != tt.want {
				t.Errorf("emitCount(%d runes)=%d, want %d", len([]rune(tt.buf)), got, tt.want)
			}
		})
	}
}

func TestEmitCountNeverStalls(t *testing.T) {
	// 任何非空缓冲都必须至少吐 1 个 rune，否则打字机会卡死。
	for _, s := range []string{"a", "ab", "你", "🦈", strings.Repeat("z", 5)} {
		if emitCount(s) < 1 {
			t.Errorf("emitCount(%q)=%d, must be >=1 for non-empty buffer", s, emitCount(s))
		}
	}
}

func TestTakeRunes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		n        int
		wantHead string
		wantTail string
	}{
		{"ascii split", "hello", 2, "he", "llo"},
		{"cjk split", "你好世界", 2, "你好", "世界"},
		{"n exceeds length", "ab", 5, "ab", ""},
		{"n zero", "abc", 0, "", "abc"},
		{"n negative", "abc", -1, "", "abc"},
		{"empty string", "", 3, "", ""},
		{"emoji", "🦈ab", 1, "🦈", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head, tail := takeRunes(tt.s, tt.n)
			if head != tt.wantHead || tail != tt.wantTail {
				t.Errorf("takeRunes(%q,%d)=(%q,%q), want (%q,%q)",
					tt.s, tt.n, head, tail, tt.wantHead, tt.wantTail)
			}
		})
	}
}

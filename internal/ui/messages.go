package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Message 表示会话历史中的一条记录。
type Message struct {
	Role      string // "user" 或 "assistant"
	Content   string
	Reasoning string // 模型的思考过程（如 DeepSeek-R1 的 reasoning_content）
}

// lineKind 标识一行渲染出的气泡行的类型。
type lineKind int

const (
	kindText lineKind = iota // 折行后的文本行（带 ▌ 边条 + 背景）
	kindPad                  // 气泡上方/下方的空背景行
	kindGap                  // 两个气泡之间的空行（透出背景图）
)

// msgLine 是会话区中渲染出的一行。
type msgLine struct {
	kind        lineKind
	msgIdx      int
	text        string
	isReasoning bool // 思考过程行，用较暗的颜色渲染
}

// streamKind 标识合并滚动流中一行的类型。
type streamKind int

const (
	streamBlank   streamKind = iota // 纯背景图行（透出背景）
	streamVersion                   // 版本号行
	streamArt                       // 横幅艺术字行（artRow 指明第几行）
	streamBubble                    // 气泡行（bubble 字段携带 msgLine）
)

// streamLine 是“logo + 气泡”合并滚动流中的一行。
type streamLine struct {
	kind   streamKind
	artRow int
	bubble msgLine
}

// buildScrollStream 把横幅（版本号 + 艺术字）与预计算的气泡行拼成一条
// 自上而下的内容流，logo 在最前，因此向下滚动时 logo 会先滑出视口。
// artRows 是艺术字的行数。
func buildScrollStream(artRows int, bubbleLines []msgLine) []streamLine {
	out := make([]streamLine, 0, artRows+len(bubbleLines)+4)
	out = append(out, streamLine{kind: streamBlank})
	out = append(out, streamLine{kind: streamVersion})
	for i := 0; i < artRows; i++ {
		out = append(out, streamLine{kind: streamArt, artRow: i})
	}
	out = append(out, streamLine{kind: streamBlank}) // logo 与气泡之间的间隙
	for _, bl := range bubbleLines {
		out = append(out, streamLine{kind: streamBubble, bubble: bl})
	}
	return out
}

// buildBubbleLines 把每条消息展开成它的气泡行：一行空白填充行、若干折行文本行、
// 一行空白填充行 —— 相邻气泡之间插入一行间隙行。textW 是文本行的折行宽度。
// 如果消息带有 Reasoning，先显示思考过程（isReasoning=true），再显示最终回答。
func buildBubbleLines(msgs []Message, textW int) []msgLine {
	var out []msgLine
	for i, m := range msgs {
		if i > 0 {
			out = append(out, msgLine{kind: kindGap, msgIdx: -1})
		}
		out = append(out, msgLine{kind: kindPad, msgIdx: i})
		if m.Reasoning != "" {
			wraps := wrapLines(m.Reasoning, textW)
			for _, w := range wraps {
				out = append(out, msgLine{kind: kindText, msgIdx: i, text: w, isReasoning: true})
			}
		}
		wraps := wrapLines(m.Content, textW)
		if len(wraps) == 0 {
			wraps = []string{""}
		}
		for _, w := range wraps {
			out = append(out, msgLine{kind: kindText, msgIdx: i, text: w})
		}
		out = append(out, msgLine{kind: kindPad, msgIdx: i})
	}
	return out
}

// wrapLines 先按换行符切分文本，再把每段折行到 maxW 宽度。
// 空段在结果中产生一个空字符串。
func wrapLines(text string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	var lines []string
	segments := strings.Split(text, "\n")
	for _, seg := range segments {
		if seg == "" {
			lines = append(lines, "")
			continue
		}
		runes := []rune(seg)
		n := len(runes)
		start := 0
		for start < n {
			breakAt := -1
			w := 0
			reachedEnd := true
			for i := start; i < n; i++ {
				rw := runewidth.RuneWidth(runes[i])
				if w+rw > maxW {
					reachedEnd = false
					break
				}
				w += rw
				if runes[i] == ' ' {
					breakAt = i
				}
			}
			if reachedEnd {
				lines = append(lines, string(runes[start:]))
				break
			}
			if breakAt <= start {
				w = 0
				for i := start; i < n; i++ {
					rw := runewidth.RuneWidth(runes[i])
					if w+rw > maxW {
						breakAt = i
						break
					}
					w += rw
					breakAt = i + 1
				}
			}
			lines = append(lines, string(runes[start:breakAt]))
			start = breakAt
			if start < n && runes[start] == ' ' {
				start++
			}
		}
	}
	return lines
}

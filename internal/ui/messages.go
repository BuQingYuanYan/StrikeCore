package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Message 表示会话历史中的一条记录。
type Message struct {
	Role    string // "user" 或 "assistant"
	Content string
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
	kind   lineKind
	msgIdx int
	text   string
}

// buildBubbleLines 把每条消息展开成它的气泡行：一行空白填充行、若干折行文本行、
// 一行空白填充行 —— 相邻气泡之间插入一行间隙行。textW 是文本行的折行宽度。
func buildBubbleLines(msgs []Message, textW int) []msgLine {
	var out []msgLine
	for i, m := range msgs {
		if i > 0 {
			out = append(out, msgLine{kind: kindGap, msgIdx: -1})
		}
		out = append(out, msgLine{kind: kindPad, msgIdx: i})
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
			if n-start <= maxW {
				lines = append(lines, string(runes[start:]))
				break
			}
			breakAt := -1
			w := 0
			for i := start; i < n; i++ {
				rw := runewidth.RuneWidth(runes[i])
				if w+rw > maxW {
					break
				}
				w += rw
				if runes[i] == ' ' {
					breakAt = i
				}
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

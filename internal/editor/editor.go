// Package editor 包提供文本输入模型及所有光标/换行逻辑。它是纯逻辑包：无终端访问、无全局变量、无渲染。渲染位于 ui 包，通过导出的访问器读取编辑器状态。
package editor

import (
	"github.com/mattn/go-runewidth"

	"strike-core/internal/input"
)

// Char 是一个 rune 及其预计算显示宽度的配对。
type Char struct {
	R rune
	W int
}

// Editor 是一个支持垂直光标移动的单字段自动换行文本编辑器。
type Editor struct {
	chars        []Char
	cursor       int
	scrollLine   int
	lastInputW   int
	cachedStarts []int
	startsDirty  bool
}

// Len 返回缓冲区中的字符数量。
func (e *Editor) Len() int { return len(e.chars) }

// Cursor 返回缓冲区中当前光标索引。
func (e *Editor) Cursor() int { return e.cursor }

// ScrollLine 返回第一个可见的换行行。
func (e *Editor) ScrollLine() int { return e.scrollLine }

// LastInputW 返回最近一次换行使用的输入宽度。
func (e *Editor) LastInputW() int { return e.lastInputW }

// SetInputW 更新换行宽度，若宽度变化则标记缓存为脏。
func (e *Editor) SetInputW(w int) {
	if w != e.lastInputW {
		e.startsDirty = true
		e.lastInputW = w
	}
}

// Slice 返回 [start,end) 范围内的字符。
func (e *Editor) Slice(start, end int) []Char { return e.chars[start:end] }

// WrapLines 返回给定输入宽度下每个换行行的起始索引，缓存结果直到缓冲区或宽度发生变化。
func (e *Editor) WrapLines(inputW int) []int {
	if !e.startsDirty && len(e.cachedStarts) > 0 {
		return e.cachedStarts
	}
	n := len(e.chars)
	capHint := max(n/max(inputW, 1), 1)
	starts := make([]int, 0, capHint)
	starts = append(starts, 0)
	w := 0
	for i := 0; i < n; i++ {
		rw := e.chars[i].W
		if w+rw > inputW {
			starts = append(starts, i)
			w = rw
		} else {
			w += rw
		}
	}
	e.cachedStarts = starts
	e.startsDirty = false
	return starts
}

// CursorPos 将光标索引映射到换行布局中的（行，列）。
func (e *Editor) CursorPos(starts []int, inputW int) (line, col int) {
	n := len(starts)
	line = n - 1
	for i := 0; i < n-1; i++ {
		if e.cursor < starts[i+1] {
			line = i
			break
		}
	}
	col = 0
	for i := starts[line]; i < e.cursor; i++ {
		col += e.chars[i].W
	}
	if col >= inputW {
		if line+1 < len(starts) {
			return line + 1, 0
		}
		return line, col
	}
	return line, col
}

// EnsureVisible 滚动以使 cursorLine 位于 inputRows 行视口内。它是纯状态计算（无渲染），提取出来以便独立于绘图进行单元测试。
func (e *Editor) EnsureVisible(cursorLine, totalLines, inputRows int) {
	maxScroll := max(0, totalLines-inputRows)
	if e.scrollLine > maxScroll {
		e.scrollLine = maxScroll
	}
	if cursorLine >= e.scrollLine+inputRows {
		e.scrollLine = cursorLine - inputRows + 1
	}
	if cursorLine < e.scrollLine {
		e.scrollLine = cursorLine
	}
}

func (e *Editor) moveVert(delta int) {
	if e.lastInputW <= 0 {
		return
	}
	starts := e.WrapLines(e.lastInputW)
	line, col := e.CursorPos(starts, e.lastInputW)
	target := line + delta
	if target < 0 {
		e.cursor = 0
		return
	}
	if target >= len(starts) {
		e.cursor = len(e.chars)
		return
	}
	start := starts[target]
	end := len(e.chars)
	if target+1 < len(starts) {
		end = starts[target+1]
	}
	w := 0
	idx := start
	for idx < end {
		rw := e.chars[idx].W
		if w+rw > col {
			break
		}
		w += rw
		idx++
	}
	e.cursor = idx
}

func (e *Editor) deleteAt(i int) {
	e.chars = append(e.chars[:i], e.chars[i+1:]...)
	e.startsDirty = true
}

// HandleKey 将解码后的按键应用到编辑器。如果按键表示退出则返回 true。
func (e *Editor) HandleKey(code int, r rune) (quit bool) {
	switch code {
	case input.KeyQuit:
		return true
	case input.KeyLeft:
		if e.cursor > 0 {
			e.cursor--
		}
	case input.KeyRight:
		if e.cursor < len(e.chars) {
			e.cursor++
		}
	case input.KeyUp:
		e.moveVert(-1)
	case input.KeyDown:
		e.moveVert(1)
	case input.KeyHome:
		e.cursor = 0
	case input.KeyEnd:
		e.cursor = len(e.chars)
	case input.KeyBackspace:
		if e.cursor > 0 {
			e.cursor--
			e.deleteAt(e.cursor)
		}
	case input.KeyDelete:
		if e.cursor < len(e.chars) {
			e.deleteAt(e.cursor)
		}
	case input.KeyRune:
		c := Char{R: r, W: runewidth.RuneWidth(r)}
		e.chars = append(e.chars[:e.cursor], append([]Char{c}, e.chars[e.cursor:]...)...)
		e.cursor++
		e.startsDirty = true
	}
	return false
}

// Clear 重置编辑器缓冲区和光标。
func (e *Editor) Clear() {
	e.chars = nil
	e.cursor = 0
	e.scrollLine = 0
	e.startsDirty = true
	e.cachedStarts = nil
}

// String 以字符串形式返回缓冲区内容。
func (e *Editor) String() string {
	runes := make([]rune, len(e.chars))
	for i, c := range e.chars {
		runes[i] = c.R
	}
	return string(runes)
}

// CharsToString 将 Char 切片渲染为字符串。
func CharsToString(chars []Char) string {
	runes := make([]rune, len(chars))
	for i, c := range chars {
		runes[i] = c.R
	}
	return string(runes)
}

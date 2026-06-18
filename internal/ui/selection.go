package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// inputLineBase 是输入框行 lineID 的基数。会话流行 = streamLine 下标，输入框行 = inputLineBase + 折行号，
// 两区域 lineID 不重叠，输入框恒在会话流之后（lineID 越大越靠下）。
const inputLineBase = 1 << 20

// SelPos 锚定逻辑位置：LineID（会话流=streamLine 下标，输入框=inputLineBase+折行号），Col（rune 索引）。
type SelPos struct {
	LineID int
	Col    int
}

// before 判断 p 是否在 q 之前（先比 LineID，再比 Col）。
func (p SelPos) before(q SelPos) bool {
	if p.LineID != q.LineID {
		return p.LineID < q.LineID
	}
	return p.Col < q.Col
}

// Selection 表示一段拖拽选区。Anchor 为按下点，Caret 为当前点，Active=false 表示无选区。
type Selection struct {
	Anchor SelPos
	Caret  SelPos
	Active bool
}

// Normalize 返回 (start, end)，start ≤ end，处理反向拖拽。
func (s Selection) Normalize() (start, end SelPos) {
	if s.Caret.before(s.Anchor) {
		return s.Caret, s.Anchor
	}
	return s.Anchor, s.Caret
}

// selectableLine 绑定逻辑行号(lineID)、本帧屏幕位置(sy/x0)和文本，供 HitTest/高亮/抽取用。
type selectableLine struct {
	lineID int
	sy     int
	x0     int
	text   string
}

// hitLine 按屏幕坐标 (sx,sy) 在可选行表中命中逻辑位置 SelPos。未命中返回 ok=false。
func hitLine(lines []selectableLine, sx, sy int) (SelPos, bool) {
	for _, ln := range lines {
		if ln.sy != sy {
			continue
		}
		rel := sx - ln.x0
		col := colAtCellX(ln.text, rel)
		return SelPos{LineID: ln.lineID, Col: col}, true
	}
	return SelPos{}, false
}

// spanForLine 返回行在选区内的高亮 rune 列区间 [c0, c1)，按首/中/末行裁剪。ok=false 表示不在选区内。
func spanForLine(sel Selection, lineID, runeLen int) (c0, c1 int, ok bool) {
	if !sel.Active {
		return 0, 0, false
	}
	start, end := sel.Normalize()
	if lineID < start.LineID || lineID > end.LineID {
		return 0, 0, false
	}
	c0, c1 = 0, runeLen
	if lineID == start.LineID {
		c0 = clampCol(start.Col, runeLen)
	}
	if lineID == end.LineID {
		c1 = clampCol(end.Col, runeLen)
	}
	if c0 > c1 {
		c0 = c1
	}
	return c0, c1, c0 < c1
}

// SelectedTextFromLines 按 lineID 顺序抽取选区文本（跨行以 \n 连接），表顺序即上下文本顺序。
func SelectedTextFromLines(lines []selectableLine, sel Selection) string {
	if !sel.Active {
		return ""
	}
	start, end := sel.Normalize()
	var b strings.Builder
	wrote := false
	for _, ln := range lines {
		if ln.lineID < start.LineID || ln.lineID > end.LineID {
			continue
		}
		runes := []rune(ln.text)
		lo, hi := 0, len(runes)
		if ln.lineID == start.LineID {
			lo = clampCol(start.Col, len(runes))
		}
		if ln.lineID == end.LineID {
			hi = clampCol(end.Col, len(runes))
		}
		if lo > hi {
			lo = hi
		}
		if wrote {
			b.WriteByte('\n')
		}
		b.WriteString(string(runes[lo:hi]))
		wrote = true
	}
	return b.String()
}

// colAtCellX 把文本内相对单元格列映到 rune 索引（按 RuneWidth 累加），宽字符右半归该字符。
func colAtCellX(text string, relCellX int) int {
	if relCellX <= 0 {
		return 0
	}
	runes := []rune(text)
	x := 0
	for i, r := range runes {
		w := runewidth.RuneWidth(r)
		if relCellX < x+w {
			return i
		}
		x += w
	}
	return len(runes)
}

// cellSpanForCols 把 rune 区间 [c0, c1) 转为显示单元格列区间。空区间返回 xStart==xEnd。
func cellSpanForCols(text string, c0, c1 int) (xStart, xEnd int) {
	runes := []rune(text)
	n := len(runes)
	if c0 < 0 {
		c0 = 0
	}
	if c1 > n {
		c1 = n
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	x := 0
	for i := 0; i < c0; i++ {
		x += runewidth.RuneWidth(runes[i])
	}
	xStart = x
	for i := c0; i < c1; i++ {
		x += runewidth.RuneWidth(runes[i])
	}
	xEnd = x
	return xStart, xEnd
}

func clampCol(c, n int) int {
	if c < 0 {
		return 0
	}
	if c > n {
		return n
	}
	return c
}

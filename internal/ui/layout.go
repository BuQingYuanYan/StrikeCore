package ui

import (
	"github.com/mattn/go-runewidth"
)

// Layout constants.
const (
	SepMargin        = 16
	BlockGap         = 3
	InputRows        = BlockGap
	InputPrompt      = "❯ "
	InputPlaceholder = "输入消息..."
	ArtRows          = 3
)

// Layout 保存某个终端尺寸下一帧的计算几何。
type Layout struct {
	Inner, Rows         int
	InputW, TextW       int
	BottomGap           int
	WorkRow, Sep2, Sep1 int
	ArtBottom, ArtTop   int
	VerRow, HintRow     int
	ArtPad              int
	MsgTop, MsgRows     int
	EdgeX               int // 左侧块边（▌）的 x 坐标；内容从 EdgeX+1 开始
}

// PromptW 是提示符字符串的显示宽度。
var PromptW = runewidth.StringWidth(InputPrompt)

// CalcLayout 计算 w×h 终端下一帧的几何。msgLines 是要显示的折行消息行数
// （0 = 无消息，布局与原始基于 BlockGap 的间距完全一致）。
//
// 内容行在顶边框（y=0）与底边框（y=Rows+1）之间从 0 开始计数。当 msgLines > 0
// 时，顶部空行与底部间隙会收缩，为会话区腾出空间。
func CalcLayout(w, h, artW int, msgLines int) Layout {
	inner := w - 2
	rows := h - 2
	if inner < 1 {
		inner = 1
	}
	if rows < 1 {
		rows = 1
	}
	inputW := max(inner-2*SepMargin, 1)
	textW := max(inputW-PromptW, 1)
	artPad := max((inner-artW)/2, 0)

	if msgLines <= 0 {
		// ── 原始布局（无消息） ──
		const fixedRows = 1 + ArtRows + BlockGap + 1 + InputRows + 1 + 1
		bottomGap := max((3*rows-3*fixedRows-3)/5, 0)
		workRow := rows - 1
		sep2 := workRow - bottomGap - 1
		sep1 := sep2 - InputRows - 1
		artBottom := sep1 - BlockGap - 1
		artTop := artBottom - (ArtRows - 1)
		verRow := artTop - 1
		hintRow := sep2 + (bottomGap+1)/2
		return Layout{
			Inner: inner, Rows: rows,
			InputW: inputW, TextW: textW,
			BottomGap: bottomGap,
			WorkRow:   workRow, Sep2: sep2, Sep1: sep1,
			ArtBottom: artBottom, ArtTop: artTop,
			VerRow: verRow, HintRow: hintRow,
			ArtPad: artPad,
			MsgTop: artBottom + 1, MsgRows: 0,
			EdgeX: SepMargin,
		}
	}

	// ── 有消息时的布局 ──
	// 顶部空 2 行，然后 version(1)、art(3)、间隙(1) = 固定 7 行。
	// 输入块被钉在底部，使其下方分隔线无论消息多少都正好位于工作目录行的上方：
	//   workRow = rows-1，sep2 = workRow-1，input 在 sep2 上方占 InputRows 行，
	//   sep1 在 input 上方。消息填充 msgTop 与 sep1 之间的空隙。
	// 有消息模式下，输入栏/气泡区两侧各留 1 列间隙贴近边框：
	//   x=1 留空，块边（▌）在 x=2，内容 x=3..Inner-1，x=Inner 留空。
	// 因此 EdgeX=2，InputW（虚线/气泡背景宽度，从 x=EdgeX+1 起绘制）= Inner-3，
	// 正好结束于 Inner-1。
	verRow := 2
	artTop := 3
	artBottom := 5
	msgTop := 7 // art 下方间隙之后的第一行内容

	edgeX := 2
	mInputW := max(inner-3, 1) // 内容宽度，从 x=3 到 x=Inner-1
	mTextW := max(mInputW-PromptW, 1)

	workRow := rows - 1
	sep2 := workRow - 1
	sep1 := max(sep2-InputRows-1, msgTop)
	msgRows := max(sep1-msgTop, 0)

	return Layout{
		Inner: inner, Rows: rows,
		InputW: mInputW, TextW: mTextW,
		BottomGap: 0,
		WorkRow:   workRow, Sep2: sep2, Sep1: sep1,
		ArtBottom: artBottom, ArtTop: artTop,
		VerRow: verRow, HintRow: 0,
		ArtPad: artPad,
		MsgTop: msgTop, MsgRows: msgRows,
		EdgeX: edgeX,
	}
}

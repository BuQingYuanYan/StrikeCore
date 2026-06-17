package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"strike-core/internal/config"
	"strike-core/internal/editor"
	"strike-core/internal/screen"
	"strike-core/internal/style"
)

// View 拥有渲染一帧所需的一切：单元格缓冲区、主题、背景、横幅数据、消息和配置。
// 所有之前的包级绘图函数现在都是方法，所有之前的全局缓存现在都是字段，
// 因此 View 是自包含的，没有共享的可变状态。
type View struct {
	screen *screen.Screen
	theme  style.Theme
	bg     *Background
	art    artData
	cfg    config.Config

	workDir         string
	promptSpaces    string
	messages        []Message
	msgScroll       int
	bubbleBgOpacity float64 // 0=纯色不透明, 1=完全透明（透出背景图）
	quitPending     bool    // 第一次 Ctrl+C 等待确认退出
	flashMsg        string  // 一次性提示，显示在输入栏占位处，有按键输入时清除

	// 按尺寸缓存的字符串
	borderCacheW int
	borderTop    string
	borderBot    string
	sepCacheW    int
	sepLine      string
	bgSpacesW    int
	bgSpaces     string
	workLineDir  string
	workLine     string
}

// NewView 从依赖项构建一个 View。
func NewView(s *screen.Screen, cfg config.Config, bg *Background, workDir string) *View {
	return &View{
		screen:       s,
		theme:        cfg.Theme,
		bg:           bg,
		art:          buildArt(cfg.AsciiArt),
		cfg:          cfg,
		workDir:      workDir,
		promptSpaces: strings.Repeat(" ", PromptW),
	}
}

// ArtWidth 返回横幅宽度供布局计算使用。
func (v *View) ArtWidth() int { return v.art.width }

// SetBubbleBgOpacity 设置气泡背景透明度（0=不透明，1=完全透明）。
func (v *View) SetBubbleBgOpacity(opacity float64) {
	v.bubbleBgOpacity = opacity
}

// SetQuitPending 设置是否处于待退出状态（第一次 Ctrl+C 后等待确认）。
func (v *View) SetQuitPending(pending bool) {
	v.quitPending = pending
}

// Flash 显示一条一次性提示在输入栏占位处，下次按键时自动清除。
func (v *View) Flash(msg string) {
	v.flashMsg = msg
}

// ClearFlash 清除一次性提示。
func (v *View) ClearFlash() {
	v.flashMsg = ""
}

// SetMessages 更新对话历史和滚动偏移。
func (v *View) SetMessages(msgs []Message, scroll int) {
	v.messages = msgs
	v.msgScroll = scroll
}

// Render 在给定编辑器状态下为终端尺寸 w×h 绘制完整帧，
// 并返回光标应处的位置。它不会刷新屏幕。
func (v *View) Render(e *editor.Editor, w, h int) style.Cursor {
	v.bg.ensure(w, h)
	if v.bg.topColors != nil {
		v.screen.FillBg(w, h, v.bg.topColors, v.bg.botColors)
	} else {
		v.screen.Clear()
	}

	inner := w - 2
	if inner < 1 {
		inner = 1
	}
	msgTextW := max(max(inner-3, 1)-PromptW, 1)

	// 仅计算一次气泡行，供布局计算和滚动流复用
	bubbleLines := buildBubbleLines(v.messages, msgTextW)
	msgLines := len(bubbleLines)
	ly := CalcLayout(w, h, v.art.width, msgLines)
	e.SetInputW(ly.TextW)

	v.drawBorder(ly)
	v.drawVerticalBorders(ly)
	v.drawWorkDir(ly)
	v.drawHint(ly)
	if len(v.messages) > 0 {
		v.drawScrollContent(ly, bubbleLines)
	} else {
		v.drawArt(ly)
	}
	v.drawSeparators(ly)
	return v.drawInput(e, ly)
}

// ScrollOffset 返回上一次 Render 钳制后的滚动偏移，供事件循环写回，
// 使“滚动到底部”（传入极大值）后再上滚能从真实底部开始。
func (v *View) ScrollOffset() int { return v.msgScroll }

func (v *View) drawBgImage(w, h int) {
	if v.bg.topColors == nil {
		return
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v.screen.SetCell(x, y, '▀', v.bg.topColors[y][x], v.bg.botColors[y][x])
		}
	}
}

func (v *View) drawText(x, y int, text string, fg, bg style.Color) int {
	for _, r := range text {
		v.screen.SetCell(x, y, r, fg, bg)
		x += runewidth.RuneWidth(r)
	}
	return x
}

// drawTextImgBg 绘制文字并透过背景图显示。
func (v *View) drawTextImgBg(x, y int, text string, fg style.Color) int {
	for _, r := range text {
		v.screen.SetCell(x, y, r, fg, v.bg.BotColor(x, y))
		x += runewidth.RuneWidth(r)
	}
	return x
}

func (v *View) getBorder(inner int) (top, bot string) {
	if inner != v.borderCacheW {
		line := strings.Repeat("─", inner)
		v.borderTop = "╭" + line + "╮"
		v.borderBot = "╰" + line + "╯"
		v.borderCacheW = inner
	}
	return v.borderTop, v.borderBot
}

func (v *View) getSepLine(w int) string {
	if w != v.sepCacheW {
		seg := "─── "
		segW := runewidth.StringWidth(seg)
		n := w / segW
		v.sepLine = strings.Repeat(seg, n) + strings.Repeat("─", w-n*segW)
		v.sepCacheW = w
	}
	return v.sepLine
}

func (v *View) getBgSpaces(w int) string {
	if w != v.bgSpacesW {
		v.bgSpaces = strings.Repeat(" ", w)
		v.bgSpacesW = w
	}
	return v.bgSpaces
}

func (v *View) getWorkLine(dir string) string {
	if dir != v.workLineDir {
		v.workLine = "[工作目录] " + dir
		v.workLineDir = dir
	}
	return v.workLine
}

func (v *View) padRight(content string, width int) string {
	cw := runewidth.StringWidth(content)
	if cw > width {
		return runewidth.Truncate(content, width, "…")
	}
	return content + v.getBgSpaces(width-cw)
}

func (v *View) drawBorder(ly Layout) {
	top, bot := v.getBorder(ly.Inner)
	v.drawTextImgBg(0, 0, top, v.theme.DimFg)
	v.drawTextImgBg(0, ly.Rows+1, bot, v.theme.DimFg)
}

func (v *View) drawVerticalBorders(ly Layout) {
	for i := 0; i < ly.Rows; i++ {
		row := i + 1
		v.screen.SetCell(0, row, '│', v.theme.DimFg, v.bg.BotColor(0, row))
		v.screen.SetCell(ly.Inner+1, row, '│', v.theme.DimFg, v.bg.BotColor(ly.Inner+1, row))
	}
}

func (v *View) drawSeparators(ly Layout) {
	sepLine := v.getSepLine(ly.InputW)
	for _, sr := range []int{ly.Sep1, ly.Sep2} {
		if sr >= 0 && sr < ly.Rows {
			sy := sr + 1
			v.screen.SetCell(ly.EdgeX, sy, '▌', v.theme.BlockEdgeFg, v.theme.BlockEdgeBg)
			v.drawText(1+ly.EdgeX, sy, sepLine, v.theme.SepFg, v.theme.InputAreaBg)
		}
	}
}

func (v *View) drawWorkDir(ly Layout) {
	if ly.WorkRow < 0 || ly.WorkRow >= ly.Rows {
		return
	}
	if len(v.messages) > 0 {
		hint := v.cfg.Hint
		dir := v.workDir
		prefix := "[StrikeCore] "
		text := prefix + dir
		// 提示右对齐，用主题色绘制，前面至少空 1 格，始终在右竖线前留 1 列间隙。
		x := 2
		x = v.drawTextImgBg(x, ly.WorkRow+1, text, v.theme.DimFg)
		remaining := ly.Inner - x // 右竖线在 Inner+1，留 1 格间隙
		if remaining > runewidth.StringWidth(hint)+1 {
			// 有足够空间：填充间隙 + 空格 + 提示
			gap := remaining - runewidth.StringWidth(hint)
			x = v.drawTextImgBg(x, ly.WorkRow+1, strings.Repeat(" ", gap), v.theme.DimFg)
			v.drawTextImgBg(x, ly.WorkRow+1, hint, v.theme.HintFg)
		} else {
			// 空间不够：只填充到 Inner，不显示提示
			v.drawTextImgBg(x, ly.WorkRow+1, strings.Repeat(" ", remaining), v.theme.DimFg)
		}
	} else {
		line := v.getWorkLine(v.workDir)
		v.drawTextImgBg(2, ly.WorkRow+1, v.padRight(line, ly.Inner-2), v.theme.DimFg)
	}
}

func (v *View) drawHint(ly Layout) {
	h := v.cfg.Hint
	if len(v.messages) > 0 {
		return // hint is rendered in the status bar
	}
	if h == "" || ly.BottomGap < 1 {
		return
	}
	if ly.HintRow < 0 || ly.HintRow >= ly.Rows {
		return
	}
	x := 1 + (ly.Inner-runewidth.StringWidth(h))/2
	if x < 1 {
		x = 1
	}
	v.drawTextImgBg(x, ly.HintRow+1, h, v.theme.HintFg)
}

func (v *View) drawArt(ly Layout) {
	for i := range v.art.texts {
		row := ly.ArtTop + i
		if row < 0 || row >= ly.Rows {
			continue
		}
		v.drawArtRowAt(i, row+1)
	}
	if ly.VerRow >= 0 {
		v.drawTextImgBg(1+ly.ArtPad, ly.VerRow+1, v.cfg.Version, v.theme.DimFg)
	}
}

// drawArtRowAt 在屏幕行 sy 处绘制第 i 行艺术字（含模型名后缀），
// 横向位置沿用 ArtPad。供居中布局和滚动流共用。
func (v *View) drawArtRowAt(i, sy int) {
	if i < 0 || i >= len(v.art.texts) {
		return
	}
	x := 1 + v.artPad()
	text := v.art.texts[i]
	leftW := v.art.leftW[i]
	if i == 1 {
		x = v.drawArtMiddleRow(x, sy, text, leftW)
	} else {
		x = v.drawArtPlainRow(x, sy, text, leftW)
	}
	if i == len(v.art.texts)-1 {
		v.drawTextImgBg(x, sy, " "+v.cfg.ModelName, v.theme.ModelFg)
	}
}

// artPad 返回横幅的左侧居中内边距，基于当前屏幕宽度计算。
func (v *View) artPad() int {
	return max((v.screen.Cols()-2-v.art.width)/2, 0)
}

func (v *View) drawArtPlainRow(x, sy int, text string, leftW int) int {
	cx := x
	for _, r := range text {
		if r != ' ' {
			fg := v.theme.ArtLeft
			if cx-x >= leftW {
				fg = v.theme.ArtRight
			}
			v.screen.SetCell(cx, sy, r, fg, v.bg.BotColor(cx, sy))
		}
		cx += runewidth.RuneWidth(r)
	}
	return cx
}

func (v *View) drawArtMiddleRow(x, sy int, text string, leftW int) int {
	cx := x
	for _, r := range text {
		rel := cx - x
		isDepth := rel < 4 ||
			(rel >= 23 && rel < 27) ||
			(rel >= leftW && rel < leftW+4) ||
			(rel >= leftW+5 && rel < leftW+9) ||
			(rel >= leftW+15 && rel < leftW+19)
		if isDepth {
			v.screen.SetCell(cx, sy, ' ', style.Color{}, v.theme.LogoDepthBg)
			if r != ' ' {
				fg := v.theme.ArtLeft
				if rel >= leftW {
					fg = v.theme.ArtRight
				}
				v.screen.SetCell(cx, sy, r, fg, v.theme.LogoDepthBg)
			}
		} else if r != ' ' {
			fg := v.theme.ArtLeft
			if rel >= leftW {
				fg = v.theme.ArtRight
			}
			v.screen.SetCell(cx, sy, r, fg, v.bg.BotColor(cx, sy))
		}
		cx += runewidth.RuneWidth(r)
	}
	return cx
}

// drawScrollContent 渲染“logo + 气泡”合并滚动流。视口是顶边框与分隔线
// 之间的内容行 [0, Sep1)（屏幕行 1..Sep1）。msgScroll 为流顶部裁掉的行数；
// 默认（极大值）钳制到底部，使最新消息可见。钳制后的偏移写回 v.msgScroll，
// 供事件循环读取，让“到底后上滚”从真实底部开始。
func (v *View) drawScrollContent(ly Layout, bubbleLines []msgLine) {
	viewRows := ly.Sep1 // 内容行 0..Sep1-1
	if viewRows <= 0 {
		return
	}
	if viewRows > ly.Rows {
		viewRows = ly.Rows
	}
	stream := buildScrollStream(len(v.art.texts), bubbleLines)
	total := len(stream)

	scroll := v.msgScroll
	maxScroll := max(0, total-viewRows)
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	v.msgScroll = scroll // 写回钳制结果

	contentX := ly.EdgeX + 1
	bgW := max(ly.Inner-ly.EdgeX-1, 0)
	opacity := v.bubbleBgOpacity

	for r := 0; r < viewRows; r++ {
		idx := scroll + r
		if idx >= total {
			break
		}
		sy := r + 1 // 屏幕行：内容行 0 在 y=1
		switch line := stream[idx]; line.kind {
		case streamBlank:
			// 透出背景图 —— 不绘制内容。
		case streamVersion:
			v.drawTextImgBg(1+v.artPad(), sy, v.cfg.Version, v.theme.DimFg)
		case streamArt:
			v.drawArtRowAt(line.artRow, sy)
		case streamBubble:
			v.drawBubbleLineAt(line.bubble, sy, contentX, bgW, opacity)
		}
	}
}

// drawBubbleLineAt 在屏幕行 sy 处绘制一条气泡行（填充行或文本行）：
// 在 EdgeX 处画块边（▌），并铺背景条。间隙行透出背景图，不绘制。
func (v *View) drawBubbleLineAt(line msgLine, sy, contentX, bgW int, opacity float64) {
	if line.kind == kindGap {
		return
	}
	bubbleBg := func(x, y int) style.Color {
		if opacity <= 0 {
			return v.theme.InputAreaBg
		}
		return v.theme.InputAreaBg.Blend(v.bg.BotColor(x, y), opacity)
	}
	edgeFg := v.theme.UserEdgeFg
	if line.msgIdx >= 0 && line.msgIdx < len(v.messages) && v.messages[line.msgIdx].Role == "assistant" {
		edgeFg = v.theme.AssistantEdgeFg
	}

	v.screen.SetCell(v.bubbleEdgeX(contentX), sy, '▌', edgeFg, bubbleBg(v.bubbleEdgeX(contentX), sy))
	if opacity <= 0 {
		if line.kind == kindText {
			v.drawText(contentX, sy, v.padRight(" "+line.text, bgW), v.theme.InputTextFg, v.theme.InputAreaBg)
		} else {
			v.drawText(contentX, sy, v.getBgSpaces(bgW), v.theme.InputTextFg, v.theme.InputAreaBg)
		}
		return
	}
	var runes []rune
	if line.kind == kindText {
		runes = []rune(v.padRight(" "+line.text, bgW))
	} else {
		runes = []rune(v.getBgSpaces(bgW))
	}
	cx := contentX
	for _, ch := range runes {
		v.screen.SetCell(cx, sy, ch, v.theme.InputTextFg, bubbleBg(cx, sy))
		cx += runewidth.RuneWidth(ch)
	}
}

// bubbleEdgeX 返回块边（▌）所在列：内容列减 1。
func (v *View) bubbleEdgeX(contentX int) int { return contentX - 1 }

// drawInput 渲染编辑器输入区域并返回光标位置。
// 滚动计算委托给编辑器；此方法仅输出单元格。
func (v *View) drawInput(e *editor.Editor, ly Layout) style.Cursor {
	wrapW := ly.TextW - 1 // 文字换行宽度，留 1 格边距
	bgW := ly.TextW       // 背景填满整个输入区
	starts := e.WrapLines(wrapW)
	cl, cc := e.CursorPos(starts, wrapW)
	e.EnsureVisible(cl, len(starts), InputRows)
	scroll := e.ScrollLine()
	var cur style.Cursor
	for r := 0; r < InputRows; r++ {
		row := ly.Sep1 + 1 + r
		if row < 0 || row >= ly.Rows {
			continue
		}
		sy := row + 1
		li := scroll + r
		v.screen.SetCell(ly.EdgeX, sy, '▌', v.theme.BlockEdgeFg, v.theme.BlockEdgeBg)
		x := 1 + ly.EdgeX
		if li == 0 {
			x = v.drawText(x, sy, InputPrompt, v.theme.PromptFg, v.theme.PromptBg)
		} else {
			x = v.drawText(x, sy, v.promptSpaces, v.theme.InputTextFg, v.theme.InputAreaBg)
		}
		if li < len(starts) {
			end := e.Len()
			if li+1 < len(starts) {
				end = starts[li+1]
			}
			if li == 0 && end == 0 {
				placeholder := InputPlaceholder
				if v.flashMsg != "" {
					placeholder = v.flashMsg
				} else if v.quitPending {
					placeholder = "再按一次退出"
				}
				v.drawText(x, sy, v.padRight(placeholder, bgW), v.theme.PlaceholderFg, v.theme.InputAreaBg)
			} else {
				text := editor.CharsToString(e.Slice(starts[li], end))
				v.drawText(x, sy, v.padRight(text, bgW), v.theme.InputTextFg, v.theme.InputAreaBg)
			}
		} else {
			v.drawText(x, sy, v.getBgSpaces(bgW), v.theme.InputTextFg, v.theme.InputAreaBg)
		}
		if li == cl {
			cur = style.Cursor{Row: sy, Col: 1 + ly.EdgeX + PromptW + cc, Visible: true}
		}
	}
	return cur
}

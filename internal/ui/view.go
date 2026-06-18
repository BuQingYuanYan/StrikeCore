package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"strike-core/internal/config"
	"strike-core/internal/editor"
	"strike-core/internal/screen"
	"strike-core/internal/style"
)

// View 拥有渲染一帧所需的一切：单元格缓冲区、主题、背景、消息、配置。
// 自包含，无共享可变状态。
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
	lastMaxScroll   int // 最近一次绘制的最大滚动偏移，用于判断用户是否在底部
	bubbleBgOpacity float64 // 0=纯色不透明, 1=完全透明（透出背景图）
	quitPending     bool    // 第一次 Ctrl+C 等待确认退出
	aborted         bool    // AI 回复刚被取消，输入栏占位显示「已终止AI答复」，几秒后或按键时清除
	flashMsg        string  // 一次性提示，显示在输入栏占位处，有按键输入时清除

	// 鲨鱼游泳动画状态
	sharkFrame  int  // 当前帧 0~5
	sharkActive bool // true = 回复中显示动画, false = 显示 [StrikeCore]

	// 鼠标选区 + 上一帧可选行表（会话流 + 输入框），每帧清空复用
	sel             Selection
	selectableLines []selectableLine

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

	// 底部提示栏覆盖文本（例如 token 消耗），非空时替代 cfg.Hint
	tokenText string
}

// SharkGradient 是鲨鱼动画 5 个位置从深到浅的海洋渐变蓝。
var SharkGradient = [5]style.Color{
	style.RGB(0x00, 0x44, 0x77),
	style.RGB(0x00, 0x66, 0xAA),
	style.RGB(0x00, 0x88, 0xCC),
	style.RGB(0x00, 0xAA, 0xEE),
	style.RGB(0x33, 0xBB, 0xFF),
}

// SetSharkFrame 设置当前鲨鱼动画帧（单调递增计数器，驱动往返循环）。
func (v *View) SetSharkFrame(n int) {
	v.sharkFrame = n
}

// sharkDisplayPos 将单调递增的帧计数器转为 0~4 往返位置：→→→→←←←←…
func sharkDisplayPos(frame int) int {
	const max = 5
	cycle := frame % (2 * (max - 1))
	if cycle >= max {
		cycle = 2*(max-1) - cycle
	}
	return cycle
}

// SharkFrame 返回当前鲨鱼动画帧。
func (v *View) SharkFrame() int {
	return v.sharkFrame
}

// SetSharkActive 控制鲨鱼动画（true=回复中，false=[StrikeCore]）。
func (v *View) SetSharkActive(active bool) {
	v.sharkActive = active
}

// SetSelection 设置当前鼠标选区，下次 Render 时据此叠加高亮。
func (v *View) SetSelection(sel Selection) {
	v.sel = sel
}

// HitTest 把屏幕坐标 (sx,sy) 反映射到 SelPos。会话流 + 输入框文本行皆可命中。
func (v *View) HitTest(sx, sy int) (SelPos, bool) {
	return hitLine(v.selectableLines, sx, sy)
}

// SelectedText 基于上一帧登记的可选行表与当前选区抽取文本。
func (v *View) SelectedText() string {
	return SelectedTextFromLines(v.selectableLines, v.sel)
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

// SetQuitPending 设置待退出状态（第一次 Ctrl+C 后）。
func (v *View) SetQuitPending(pending bool) {
	v.quitPending = pending
}

// SetAborted 设置 AI 回复是否刚被取消（输入栏显示「已终止AI答复」）。
func (v *View) SetAborted(aborted bool) {
	v.aborted = aborted
}

// Flash 显示一次性提示在输入栏占位处，按键自动清除。
func (v *View) Flash(msg string) {
	v.flashMsg = msg
}

// ClearFlash 清除一次性提示。
func (v *View) ClearFlash() {
	v.flashMsg = ""
}

// SetTokenText 设置底部提示栏覆盖文本（例如 token 消耗），
// 非空时 drawWorkDir 用此替代 cfg.Hint。传入空字符串恢复默认提示。
func (v *View) SetTokenText(text string) {
	v.tokenText = text
}

// SetMessages 更新对话历史和滚动偏移。
func (v *View) SetMessages(msgs []Message, scroll int) {
	// 过滤掉完全为空的 assistant 消息（预创建的空白占位，尚未收到流式内容）
	filtered := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "assistant" && m.Content == "" && m.Reasoning == "" {
			continue
		}
		filtered = append(filtered, m)
	}
	v.messages = filtered
	v.msgScroll = scroll
}

// Render 为终端尺寸 w×h 绘制完整帧，返回光标位置。不刷新屏幕。
func (v *View) Render(e *editor.Editor, w, h int) style.Cursor {
	v.bg.ensure(w, h)
	if v.bg.topColors != nil {
		v.screen.FillBg(w, h, v.bg.topColors, v.bg.botColors)
	} else {
		v.screen.Clear()
	}

	// 每帧重建可选行表；drawScrollContent / drawInput 会向其登记可选文本行。
	v.selectableLines = v.selectableLines[:0]

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

// ScrollOffset 返回上一次 Render 钳制后的滚动偏移，供事件循环写回，使「到底后上滚」从真实底部开始。
func (v *View) ScrollOffset() int { return v.msgScroll }

// MaxScroll 返回最近一次绘制时的最大滚动偏移（maxScroll），用于判断用户是否已在底部。
func (v *View) MaxScroll() int { return v.lastMaxScroll }

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
		dir := v.workDir
		x := 2
		if v.sharkActive {
			x = v.drawSharkAnimation(x, ly.WorkRow+1)
		} else {
			x = v.drawTextImgBg(x, ly.WorkRow+1, "[StrikeCore]", v.theme.DimFg)
		}
		x = v.drawTextImgBg(x, ly.WorkRow+1, " "+dir, v.theme.DimFg)
		remaining := ly.Inner - x
		if v.tokenText != "" && remaining > runewidth.StringWidth(v.tokenText)+1 {
			gap := remaining - runewidth.StringWidth(v.tokenText)
			x = v.drawTextImgBg(x, ly.WorkRow+1, strings.Repeat(" ", gap), v.theme.DimFg)
			v.drawTextImgBg(x, ly.WorkRow+1, v.tokenText, v.theme.HintFg)
		} else {
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

// drawArtRowAt 在屏幕行 sy 处绘制横幅第 i 行（含模型名后缀）。供居中布局和滚动流共用。
func (v *View) drawArtRowAt(i, sy int) {
	if i < 0 || i >= len(v.art.texts) {
		return
	}
	x := 1 + v.artPad()
	text := v.art.texts[i]
	leftW := v.art.leftW[i]
	solidBg := i >= len(v.art.texts)-2
	if solidBg {
		for cx := x - 1; cx <= x+v.art.width; cx++ {
			v.screen.SetCell(cx, sy, ' ', style.Color{}, v.theme.LogoDepthBg)
		}
	}
	if i == 1 {
		x = v.drawArtMiddleRow(x, sy, text, leftW, solidBg)
	} else {
		x = v.drawArtPlainRow(x, sy, text, leftW, solidBg)
	}
	if i == len(v.art.texts)-1 && v.cfg.ModelName != "" {
		if solidBg {
			v.drawTextImgBg(x+1, sy, v.cfg.ModelName, v.theme.ModelFg)
		} else {
			v.drawTextImgBg(x, sy, " "+v.cfg.ModelName, v.theme.ModelFg)
		}
	}
}

// drawSharkAnimation 在 (x,sy) 处绘制鲨鱼动画 [==🦈======]，frame 指定位置。
func (v *View) drawSharkAnimation(x, sy int) int {
	fg := v.theme.DimFg
	v.screen.SetCell(x, sy, '[', fg, v.bg.BotColor(x, sy))
	x += runewidth.RuneWidth('[')
	frame := sharkDisplayPos(v.sharkFrame)
	for pos := 0; pos < 5; pos++ {
		if pos == frame {
			v.screen.SetCell(x, sy, '🦈', style.RGB(0xFF, 0xFF, 0xFF), v.bg.BotColor(x, sy))
			x += runewidth.RuneWidth('🦈')
		} else {
			v.screen.SetCell(x, sy, '=', SharkGradient[pos], v.bg.BotColor(x, sy))
			x++
			v.screen.SetCell(x, sy, '=', SharkGradient[pos], v.bg.BotColor(x, sy))
			x++
		}
	}
	v.screen.SetCell(x, sy, ']', fg, v.bg.BotColor(x, sy))
	return x + runewidth.RuneWidth(']')
}

// artPad 返回横幅的左侧居中内边距，基于当前屏幕宽度计算。
func (v *View) artPad() int {
	return max((v.screen.Cols()-2-v.art.width)/2, 0)
}

func (v *View) drawArtPlainRow(x, sy int, text string, leftW int, solidBg bool) int {
	cx := x
	for _, r := range text {
		if r != ' ' {
			fg := v.theme.ArtLeft
			if cx-x >= leftW {
				fg = v.theme.ArtRight
			}
			bg := v.bg.BotColor(cx, sy)
			if solidBg {
				bg = v.theme.LogoDepthBg
			}
			v.screen.SetCell(cx, sy, r, fg, bg)
		}
		cx += runewidth.RuneWidth(r)
	}
	return cx
}

func (v *View) drawArtMiddleRow(x, sy int, text string, leftW int, solidBg bool) int {
	cx := x
	for _, r := range text {
		rel := cx - x
		isDepth := solidBg ||
			rel < 4 ||
			(rel >= 23 && rel < 27) ||
			(rel >= leftW && rel < leftW+4) ||
			(rel >= leftW+5 && rel < leftW+9) ||
			(rel >= leftW+15 && rel < leftW+19)
		bg := v.theme.LogoDepthBg
		if !isDepth {
			bg = v.bg.BotColor(cx, sy)
		}
		v.screen.SetCell(cx, sy, ' ', style.Color{}, bg)
		if r != ' ' {
			fg := v.theme.ArtLeft
			if rel >= leftW {
				fg = v.theme.ArtRight
			}
			v.screen.SetCell(cx, sy, r, fg, bg)
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
	viewRows := max(ly.Sep1-1, 0) // 留一行透出壁纸，在气泡与输入栏之间形成隔断
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
	v.lastMaxScroll = maxScroll

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
			// 登记可选会话流文本行：lineID=流下标 idx（远小于 inputLineBase，恒排在
			// 输入框行之前）；文本经 padRight(" "+text) 有前缀空格，真实文本 1-based
			// 屏幕起列 = contentX+2（contentX 为 0-based 内容列）。
			if line.bubble.kind == kindText {
				v.selectableLines = append(v.selectableLines, selectableLine{
					lineID: idx,
					sy:     sy,
					x0:     contentX + 2,
					text:   line.bubble.text,
				})
			}
			v.drawBubbleLineAt(line.bubble, idx, sy, contentX, bgW, opacity)
		}
	}
}

// drawBubbleLineAt 在屏幕行 sy 处绘制气泡行（块边 ▌ + 背景），间隙行透出背景图。
// idx 是该行流下标（= 会话流行的 lineID），用于选区高亮。
func (v *View) drawBubbleLineAt(line msgLine, idx, sy, contentX, bgW int, opacity float64) {
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

	textFg := v.theme.InputTextFg
	if line.isReasoning {
		textFg = v.theme.DimFg
	}
	if line.isReasoningLabel {
		textFg = v.theme.HintFg
	}

	// 选区高亮：把 rune 列区间转成相对文本起列(contentX+1)的显示单元格区间。
	// 仅文本行可选；填充行不高亮。
	selXStart, selXEnd := 0, 0
	if line.kind == kindText {
		if c0, c1, ok := spanForLine(v.sel, idx, len([]rune(line.text))); ok {
			xs, xe := cellSpanForCols(line.text, c0, c1)
			tx0 := contentX + 1 // " "+text 的前缀空白占一格
			selXStart, selXEnd = tx0+xs, tx0+xe
		}
	}
	inSel := func(x int) bool { return x >= selXStart && x < selXEnd }

	v.screen.SetCell(v.bubbleEdgeX(contentX), sy, '▌', edgeFg, bubbleBg(v.bubbleEdgeX(contentX), sy))

	var runes []rune
	if line.kind == kindText {
		runes = []rune(v.padRight(" "+line.text, bgW))
	} else {
		runes = []rune(v.getBgSpaces(bgW))
	}
	cx := contentX
	for _, ch := range runes {
		bg := bubbleBg(cx, sy)
		if inSel(cx) {
			bg = v.theme.SelectionBg
		}
		v.screen.SetCell(cx, sy, ch, textFg, bg)
		cx += runewidth.RuneWidth(ch)
	}
}

// bubbleEdgeX 返回块边（▌）所在列：内容列减 1。
func (v *View) bubbleEdgeX(contentX int) int { return contentX - 1 }

// drawInputTextLine 在屏幕行 sy、起列 x 处绘制输入框文本行（含背景填充 + 选区高亮）。
// lineID = inputLineBase+li，用于查选区列区间。
func (v *View) drawInputTextLine(x, sy int, text string, lineID, bgW int) {
	selXStart, selXEnd := 0, 0
	if c0, c1, ok := spanForLine(v.sel, lineID, len([]rune(text))); ok {
		xs, xe := cellSpanForCols(text, c0, c1)
		selXStart, selXEnd = x+xs, x+xe
	}
	inSel := func(cx int) bool { return cx >= selXStart && cx < selXEnd }

	runes := []rune(v.padRight(text, bgW))
	cx := x
	for _, ch := range runes {
		bg := v.theme.InputAreaBg
		if inSel(cx) {
			bg = v.theme.SelectionBg
		}
		v.screen.SetCell(cx, sy, ch, v.theme.InputTextFg, bg)
		cx += runewidth.RuneWidth(ch)
	}
}

// drawInput 渲染编辑器输入区域并返回光标位置。滚动委托给编辑器，此方法仅输出单元格。
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
				} else if v.aborted {
					placeholder = "已终止AI答复"
				} else if v.quitPending {
					placeholder = "再按一次退出"
				}
				v.drawText(x, sy, v.padRight(placeholder, bgW), v.theme.PlaceholderFg, v.theme.InputAreaBg)
			} else {
				text := editor.CharsToString(e.Slice(starts[li], end))
				// 登记可选输入框文本行：lineID=inputLineBase+li（恒大于会话流行下标，
				// 故排在会话流之后）。x 是 0-based 屏幕绘制列，对应 1-based 文本起列 x+1；
				// HitTest 的 sx 为 1-based，故 x0=x+1 才使命中与字符精确对齐。
				lineID := inputLineBase + li
				v.selectableLines = append(v.selectableLines, selectableLine{
					lineID: lineID,
					sy:     sy,
					x0:     x + 1,
					text:   text,
				})
				v.drawInputTextLine(x, sy, text, lineID, bgW)
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

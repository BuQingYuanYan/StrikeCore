// Package app 将终端、屏幕和视图组合在一起并运行事件循环。
// 它管理进程级别的问题：原始模式设置/恢复、恐慌恢复、信号处理以及通过上下文管理协程生命周期。
package app

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"strike-core/internal/clipboard"
	"strike-core/internal/config"
	"strike-core/internal/editor"
	"strike-core/internal/input"
	"strike-core/internal/llm"
	"strike-core/internal/screen"
	"strike-core/internal/style"
	"strike-core/internal/terminal"
	"strike-core/internal/ui"
)

// VT 控制序列用于会话设置/拆卸。这些是可移植的，不是终端后端关心的问题。
const (
	enterAltScreen = "\x1b[?1049h"
	leaveAltScreen = "\x1b[?1049l"
	hideCursor     = "\x1b[?25l"
	showCursor     = "\x1b[?25h"
	clearScreen    = "\x1b[2J"
	cursorHome     = "\x1b[H"
	// ?1000h=点击上报  ?1002h=按住拖拽上报（自绘拖选必需，不用 ?1003h 以免刷爆事件）
	// ?1006h=SGR 扩展坐标（支持大坐标）
	enableMouse  = "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
	disableMouse = "\x1b[?1006l\x1b[?1002l\x1b[?1000l"
)

// scrollStep 是一次滚轮事件滚动的行数。
const scrollStep = 3

// scrollToBottom 是 msgScroll 的哨兵值：表示“滚到最底部”。
// View.Render 会把它钳制到实际的 maxScroll，再经 ScrollOffset() 写回真实偏移。
const scrollToBottom = math.MaxInt

// 打字机输出参数。每个 tick 由 emitOnce 吐出若干字符；吐字数随积压量动态
// 增长，使 UI 永远追得上模型，同时在积压少时仍保留逐字动画手感。
const (
	emitInterval = 33 * time.Millisecond // 文字输出 tick 周期（约 30fps）
	emitMinRunes = 1                     // 每 tick 至少吐 1 个字（逐字手感）
	emitDivisor  = 6                     // 吐字数 = 积压量 / emitDivisor（自适应提速）
	emitMaxRunes = 64                    // 单 tick 上限，防止一次性灌爆造成跳跃
)

// sharkInterval 是鲨鱼动画推进一帧的周期，与文字输出节奏独立，
// 保持优化前的 ~5fps 手感（原实现为 100ms tick × 每 2 帧）。
const sharkInterval = 200 * time.Millisecond

// Run 设置终端并运行 UI，直到用户退出或信号到达。即使发生恐慌或收到信号，终端也始终会恢复。
// provider 可为 nil，此时使用内置的硬编码回复（离线/开发模式）。
func Run(cfg config.Config, dataDir string, workDir string, provider llm.Provider) (err error) {
	// CJK 布局依赖于一致的宽度计算；显式固定以便行为不会随环境变化。
	runewidth.DefaultCondition.EastAsianWidth = false

	term, err := terminal.New()
	if err != nil {
		return fmt.Errorf("app: init terminal: %w", err)
	}

	restore, err := term.Init()
	if err != nil {
		return fmt.Errorf("app: enter raw mode: %w", err)
	}

	out := term.Out()
	enterSession(out)

	// 确保在每个退出路径上都执行拆卸，包括恐慌时。
	defer func() {
		leaveSession(out)
		restore()
		if r := recover(); r != nil {
			err = fmt.Errorf("app: panic: %v", r)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, interruptSignals()...)
	defer signal.Stop(sig)

	s := screen.New(out)
	bgDir := filepath.Join(dataDir, "backgrounds")

	// 解析初始背景图像。backgrounds/config.json 可以覆盖活动壁纸、切换幻灯片并设置间隔。
	var (
		bgImages   []string
		bgDirCfg   = ui.ReadBgDirCfg(bgDir)
		slideReady bool
	)
	if cfg.BgPath == "" {
		bgImages = ui.ListBgImages(bgDir)
		if len(bgImages) > 0 {
			// 遵守目录配置中的壁纸覆盖。
			if bgDirCfg.Wallpaper != nil && *bgDirCfg.Wallpaper != "" {
				cfg.BgPath = filepath.Join(bgDir, *bgDirCfg.Wallpaper)
				if _, err := os.Stat(cfg.BgPath); err != nil {
					cfg.BgPath = bgImages[0]
				}
			} else {
				cfg.BgPath = bgImages[0]
			}
		}
	}
	// backgrounds/config.json 的 brightness 覆盖主配置默认值，使启动时的亮度
	// 与运行 /reload 后保持一致（否则启动用 0.35、reload 后却跳到此处的值）。
	if bgDirCfg.Brightness != nil {
		cfg.Brightness = *bgDirCfg.Brightness
	}
	bg := ui.NewBackground(cfg.BgPath, cfg.Brightness)
	view := ui.NewView(s, cfg, bg, workDir)
	if bgDirCfg.BubbleBgOpacity != nil {
		view.SetBubbleBgOpacity(*bgDirCfg.BubbleBgOpacity)
	}

	// 轻量 token 估算：ASCII 按 字符数×0.3，非 ASCII 按 ×0.6。
	// 全离线、零依赖，用作厂商返回 usage 前的流中实时值。
	countTokens := func(text string) int {
		n := 0
		for _, r := range text {
			if r <= 0x7F {
				n += 3 // ×0.3
			} else {
				n += 6 // ×0.6
			}
		}
		return n / 10
	}
	ed := &editor.Editor{}
	st := &chatState{}
	// 文字输出与鲨鱼动画用两个独立的 ticker，互不干扰：
	// emitTicker 快速吐字（追平模型），sharkTicker 固定 ~5fps 推进动画帧，
	// 这样吐字提速不会让动画节奏抖动。
	var (
		emitTicker      *time.Ticker
		emitCh          <-chan time.Time
		sharkTicker     *time.Ticker
		sharkCh         <-chan time.Time
		aiCancel        context.CancelFunc // 取消当前 AI 请求的子 context；nil 表示无进行中的请求
		aiTimeoutTimer  *time.Timer
		aiTimeoutCh     <-chan time.Time
		// AI 流式响应通道。每次请求一条新通道：取消旧请求后其 goroutine 仍会
		// close 自己那条旧通道，但事件循环只读取当前 streamCh，因此旧通道的关闭
		// 或残留 delta 不会污染新一轮请求。idle 时为 nil（nil 通道永久阻塞，符合预期）。
		streamCh chan ui.Message
	)
	stopStreamTickers := func() {
		if emitTicker != nil {
			emitTicker.Stop()
			emitTicker = nil
			emitCh = nil
		}
		if sharkTicker != nil {
			sharkTicker.Stop()
			sharkTicker = nil
			sharkCh = nil
		}
	}

	render := func() {
		w, h, e := term.Size()
		if e != nil {
			return
		}
		if w != s.Cols() || h != s.Rows() {
			s.Realloc(w, h)
		}
		if w < 4 || h < 4 {
			s.Clear()
			s.Flush(style.Cursor{Visible: false})
			return
		}
		view.SetMessages(st.messages, st.scroll)
		view.SetSelection(st.sel)
		cur := view.Render(ed, w, h)
		// Render 会把 scroll 钳制到合法范围（尤其是发送消息后设的
		// “滚动到底部”哨兵值）；读回钳制结果，使后续滚轮上滚从真实底部开始。
		st.scroll = view.ScrollOffset()
		st.lastMaxScroll = view.MaxScroll()
		s.Flush(cur)
	}

	// emitOnce 只负责文字输出：每 tick 吐出若干字符（数量随积压动态增长），
	// 缓冲清空且上游流结束时收尾、停掉两个 ticker。不再驱动鲨鱼动画。
	emitOnce := func() {
		setTokenText := func(emoji string, current int, currentEst bool, total int, totalEst bool) {
			cur := ""
			if currentEst {
				cur = fmt.Sprintf("%s ~%d", emoji, current)
			} else {
				cur = fmt.Sprintf("%s %d", emoji, current)
			}
			if total > 0 {
				tot := ""
				if totalEst {
					tot = fmt.Sprintf("∑ ~%d", total)
				} else {
					tot = fmt.Sprintf("∑ %d", total)
				}
				view.SetTokenText(cur + " · " + tot + " tokens")
			} else {
				view.SetTokenText(cur + " tokens")
			}
		}
		if st.bufReasoning != "" {
			if st.thinkingStart.IsZero() {
				st.thinkingStart = time.Now()
			}
			emitted, rest := takeRunes(st.bufReasoning, emitCount(st.bufReasoning))
			appendStreamReasoning(&st.messages, emitted)
			st.bufReasoning = rest
			if !st.scrollLocked {
				st.jumpToBottom()
			}
			render()
		} else if st.bufContent != "" {
			emitted, rest := takeRunes(st.bufContent, emitCount(st.bufContent))
			appendStreamContent(&st.messages, emitted)
			st.bufContent = rest
			if !st.scrollLocked {
				st.jumpToBottom()
			}
			render()
		} else if st.aiPending {
			if st.streamDone {
				storeThinkingDuration(&st.messages, st.thinkingStart)
				st.thinkingStart = time.Time{}
				if st.tokenFinal {
					st.sessionTotal += st.tokenTotal
				} else {
					st.sessionTotal += st.promptTokens + st.completionTokens
				}
				st.requestCount++
				total := 0
				if st.requestCount > 1 {
					total = st.sessionTotal
				}
				if st.tokenFinal {
					setTokenText("🚀", st.tokenTotal, false, total, false)
				} else {
					setTokenText("🚀", st.promptTokens+st.completionTokens, false, total, false)
				}
				st.aiPending = false
				st.streamDone = false
				st.escPendingAt = time.Time{}
				view.SetSharkActive(false)
				stopStreamTickers()
				if aiTimeoutTimer != nil {
					aiTimeoutTimer.Stop()
					aiTimeoutTimer = nil
					aiTimeoutCh = nil
				}
				if aiCancel != nil {
					aiCancel()
					aiCancel = nil
				}
				streamCh = nil
			} else {
				cur := st.promptTokens + st.completionTokens
				if st.requestCount > 0 {
					setTokenText("🚀", cur, false, st.sessionTotal+cur, true)
				} else {
					setTokenText("🚀", cur, false, 0, false)
				}
			}
			render()
			return
		}
		if st.aiPending {
			cur := st.promptTokens + st.completionTokens
			if st.requestCount > 0 {
				setTokenText("🚀", cur, false, st.sessionTotal+cur, true)
			} else {
				setTokenText("🚀", cur, false, 0, false)
			}
		}
	}

	// sharkTick 只负责推进鲨鱼动画帧，节奏由独立的 200ms ticker 决定，
	// 与文字输出速度完全解耦，确保动画不随吐字快慢抖动。
	sharkTick := func() {
		view.SetSharkFrame(view.SharkFrame() + 1)
		render()
	}

	render()

	// 壁纸幻灯片放映——定期切换到下一张图像。
	// 下一张图像在后台协程中预先解码，因此实际切换是即时的，在调整大小期间不会卡顿。
	var (
		bgSlideTicker *time.Ticker
		bgSlideCh     <-chan time.Time
		bgIndex       int
		// 壁纸渐入渐出过渡
		bgFadeTicker  *time.Ticker
		bgFadeCh      <-chan time.Time
		bgFadeStart   time.Time
		bgFadeTargetBri float64
		bgFadeNextIdx   int
		bgFadeMidpoint  bool
	)
	if bgDirCfg.Enabled != nil && !*bgDirCfg.Enabled {
		slideReady = false
	} else {
		slideReady = true
		// 目录配置间隔覆盖全局默认值。
		if bgDirCfg.Interval != nil && *bgDirCfg.Interval > 0 {
			cfg.BgInterval = time.Duration(*bgDirCfg.Interval) * time.Second
		}
	}
	if slideReady && cfg.BgInterval > 0 {
		if bgImages == nil {
			bgImages = ui.ListBgImages(bgDir)
		}
		if len(bgImages) > 1 {
			for i, p := range bgImages {
				if p == cfg.BgPath {
					bgIndex = i
					break
				}
			}
			// 预先解码第二张图像，以便第一个计时器触发立即生效。
			nextIdx := (bgIndex + 1) % len(bgImages)
			bg.Preload(bgImages[nextIdx])
			bgSlideTicker = time.NewTicker(cfg.BgInterval)
			bgSlideCh = bgSlideTicker.C
		}
	}
	defer func() {
		if bgSlideTicker != nil {
			bgSlideTicker.Stop()
		}
		if bgFadeTicker != nil {
			bgFadeTicker.Stop()
		}
	}()

	inputCh := make(chan []byte, 128)
	resizeCh := make(chan struct{}, 1)
	go readInput(ctx, term, inputCh, resizeCh)
	go watchResize(ctx, term, resizeCh)

	var (
		pending       []byte
		debounceTimer *time.Timer
		debounceCh    <-chan time.Time
		quitTimer     *time.Timer
		quitTimerCh   <-chan time.Time
		abortTimer    *time.Timer
		abortCh       <-chan time.Time
	)
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		if quitTimer != nil {
			quitTimer.Stop()
		}
		if aiTimeoutTimer != nil {
			aiTimeoutTimer.Stop()
		}
		if abortTimer != nil {
			abortTimer.Stop()
		}
		if aiCancel != nil {
			aiCancel()
		}
		stopStreamTickers()
	}()

	// cancelAI 取消进行中的 AI 回复：终止子 context、冲刷已收到的缓冲（保留已生成内容）、
	// 追加中断标记、收尾动画与计时器，并在输入栏占位处提示「已中断」（3 秒后自动恢复）。
	// 若 AI 尚未输出任何内容，则不追加终止气泡，且清理预创建的空 assistant 消息。
	cancelAI := func() {
		if !st.aiPending {
			return
		}
		if aiCancel != nil {
			aiCancel()
			aiCancel = nil
		}
		streamCh = nil

		hasPendingContent := st.bufContent != "" || st.bufReasoning != ""
		hasEmittedContent := false
		if len(st.messages) > 0 {
			last := st.messages[len(st.messages)-1]
			if last.Role == "assistant" && (last.Content != "" || last.Reasoning != "") {
				hasEmittedContent = true
			}
		}
		hasAnyContent := hasPendingContent || hasEmittedContent
		if !hasAnyContent && len(st.messages) > 0 {
			last := st.messages[len(st.messages)-1]
			if last.Role == "assistant" && last.Content == "" && last.Reasoning == "" {
				st.messages = st.messages[:len(st.messages)-1]
			}
		}

		if hasAnyContent {
			storeThinkingDuration(&st.messages, st.thinkingStart)
			st.thinkingStart = time.Time{}
		}

		if st.bufReasoning != "" {
			appendStreamReasoning(&st.messages, st.bufReasoning)
			st.bufReasoning = ""
		}
		if st.bufContent != "" {
			appendStreamContent(&st.messages, st.bufContent)
			st.bufContent = ""
		}
		if hasAnyContent {
			termMarker := "\n⏹ 已终止"
			if len(st.messages) > 0 {
				last := st.messages[len(st.messages)-1]
				if last.Role == "assistant" && last.Content == "" {
					termMarker = "⏹ 已终止"
				}
			}
			appendStreamContent(&st.messages, termMarker)
		}
		st.aiPending = false
		st.streamDone = false
		st.escPendingAt = time.Time{}
		view.SetSharkActive(false)
		stopStreamTickers()
		if aiTimeoutTimer != nil {
			aiTimeoutTimer.Stop()
			aiTimeoutTimer = nil
			aiTimeoutCh = nil
		}
		view.SetAborted(true)
		if abortTimer != nil {
			abortTimer.Stop()
		}
		abortTimer = time.NewTimer(3 * time.Second)
		abortCh = abortTimer.C
		est := st.promptTokens + st.completionTokens
		st.sessionTotal += est
		st.requestCount++
		if st.requestCount > 1 {
			view.SetTokenText(fmt.Sprintf("🚫 %d · ∑ %d tokens", est, st.sessionTotal))
		} else {
			view.SetTokenText(fmt.Sprintf("🚫 %d tokens", est))
		}
		st.jumpToBottom()
		render()
	}

	// clearSelection 清空选区高亮（开始输入、复制后、点击空白处等场景）。
	clearSelection := func() bool {
		if st.sel.Active || st.selecting {
			st.sel = ui.Selection{}
			st.selecting = false
			return true
		}
		return false
	}

	// copySelection 把当前选区文本经 OSC52 写入剪贴板，提示已复制并清空高亮。
	// 无选区或选区为空则返回 false（调用方据此回退到原 Ctrl+C 语义）。
	copySelection := func() bool {
		if !st.sel.Active {
			return false
		}
		text := view.SelectedText()
		if text == "" {
			return false
		}
		io.WriteString(out, clipboard.Encode(text))
		view.Flash(fmt.Sprintf("已复制 %d 字", utf8.RuneCountInString(text)))
		st.sel = ui.Selection{}
		st.selecting = false
		return true
	}

	// handleMouse 解析 pending 头部的 SGR 鼠标序列并更新选区/滚动状态。
	// 返回消耗的字节数 n（n==0 表示序列不完整，需等更多字节）与是否需要重绘。
	// 滚轮仍走原有 st.scrollUp/scrollDown；press/drag 经 HitTest 更新选区。
	handleMouse := func() (n int, dirty bool) {
		ev, ok, consumed := input.ParseSGRMouse(pending)
		if consumed == 0 {
			return 0, false // 不完整
		}
		if !ok {
			return consumed, false // 畸形，已跳过
		}
		switch ev.Kind {
		case input.MouseWheelUp:
			return consumed, st.scrollUp(scrollStep)
		case input.MouseWheelDown:
			return consumed, st.scrollDown(scrollStep)
		case input.MousePress:
			if pos, hit := view.HitTest(ev.X+1, ev.Y+1); hit {
				st.sel = ui.Selection{Anchor: pos, Caret: pos, Active: true}
				st.selecting = true
				return consumed, true
			}
			// 点击非内容区：清空已有选区，不开始新选区。
			return consumed, clearSelection()
		case input.MouseDrag:
			if !st.selecting {
				return consumed, false
			}
			if pos, hit := view.HitTest(ev.X+1, ev.Y+1); hit {
				st.sel.Caret = pos
				st.sel.Active = true
				return consumed, true
			}
			return consumed, false
		case input.MouseRelease:
			if st.selecting {
				st.selecting = false
				// 选区保留高亮，等待按 Ctrl+C 复制。
			}
			return consumed, false
		default:
			return consumed, false
		}
	}

	for {
		select {
		case <-sig:
			return nil
		case <-resizeCh:
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(80 * time.Millisecond)
				debounceCh = debounceTimer.C
			} else {
				debounceTimer.Stop()
				select {
				case <-debounceTimer.C:
				default:
				}
				debounceTimer.Reset(80 * time.Millisecond)
			}
		case <-debounceCh:
			render()
			debounceTimer = nil
			debounceCh = nil
			continue
		case <-bgSlideCh:
			if debounceTimer != nil {
				continue // 正在调整大小——跳过以避免卡顿
			}
			bgSlideTicker.Stop()
			bgSlideTicker = nil
			bgSlideCh = nil
			bgFadeNextIdx = (bgIndex + 1) % len(bgImages)
			bgFadeStart = time.Now()
			bgFadeMidpoint = false
			bgFadeTargetBri = bg.Brightness()
			bgFadeTicker = time.NewTicker(16 * time.Millisecond)
			bgFadeCh = bgFadeTicker.C
		case <-bgFadeCh:
			if bgFadeTicker == nil {
				continue
			}
			if bgSlideCh != nil {
				bgFadeTicker.Stop()
				bgFadeTicker = nil
				bgFadeCh = nil
				continue
			}
			elapsed := time.Since(bgFadeStart)
			half := 250 * time.Millisecond
			target := bgFadeTargetBri
			if elapsed < half {
				p := float64(elapsed) / float64(half)
				bg.SetBrightness(target * (1 - p))
			} else if !bgFadeMidpoint {
				bgFadeMidpoint = true
				bgIndex = bgFadeNextIdx
				if !bg.Activate(bgImages[bgIndex]) {
					bg.Load(bgImages[bgIndex])
				}
				nextIdx := (bgIndex + 1) % len(bgImages)
				bg.Preload(bgImages[nextIdx])
				bg.SetBrightness(0)
			}
			if elapsed >= half {
				p := float64(elapsed-half) / float64(half)
				if p > 1 {
					p = 1
				}
				bg.SetBrightness(target * p)
			}
			render()
			if elapsed >= 2*half {
				bgFadeTicker.Stop()
				bgFadeTicker = nil
				bgFadeCh = nil
				bg.SetBrightness(target)
				if bgImages != nil && len(bgImages) > 1 {
					nextIdx := (bgIndex + 1) % len(bgImages)
					bg.Preload(bgImages[nextIdx])
					bgSlideTicker = time.NewTicker(cfg.BgInterval)
					bgSlideCh = bgSlideTicker.C
				}
			}
			continue
		case <-quitTimerCh:
			st.quitPending = false
			view.SetQuitPending(false)
			quitTimer = nil
			quitTimerCh = nil
			render()
			continue
		case <-abortCh:
			view.SetAborted(false)
			abortTimer = nil
			abortCh = nil
			render()
			continue
		case <-emitCh:
			emitOnce()
			continue
		case <-sharkCh:
			sharkTick()
			continue
		case msg, ok := <-streamCh:
			if ok {
				if aiTimeoutTimer != nil {
					aiTimeoutTimer.Stop()
					aiTimeoutTimer = nil
					aiTimeoutCh = nil
				}
				if msg.TokenUsage != nil {
					st.tokenTotal = msg.TokenUsage.TotalTokens
					st.tokenFinal = true
				}
				if msg.Reasoning != "" {
					st.bufReasoning += msg.Reasoning
					st.completionTokens += countTokens(msg.Reasoning)
				}
				if msg.Content != "" {
					st.bufContent += msg.Content
					st.completionTokens += countTokens(msg.Content)
				}
			} else {
				st.streamDone = true
			}
			continue
		case <-aiTimeoutCh:
			st.aiPending = false
			stopStreamTickers()
			view.SetSharkActive(false)
			if aiCancel != nil {
				aiCancel()
				aiCancel = nil
			}
			streamCh = nil
			aiTimeoutTimer = nil
			aiTimeoutCh = nil
			hasPendingContent := st.bufContent != "" || st.bufReasoning != ""
			hasEmittedContent := false
			if len(st.messages) > 0 {
				last := st.messages[len(st.messages)-1]
				if last.Role == "assistant" && (last.Content != "" || last.Reasoning != "") {
					hasEmittedContent = true
				}
			}
			hasAnyContent := hasPendingContent || hasEmittedContent
			if !hasAnyContent && len(st.messages) > 0 {
				last := st.messages[len(st.messages)-1]
				if last.Role == "assistant" && last.Content == "" && last.Reasoning == "" {
					st.messages = st.messages[:len(st.messages)-1]
				}
			}
			if hasAnyContent {
				storeThinkingDuration(&st.messages, st.thinkingStart)
				st.thinkingStart = time.Time{}
			}
			if st.bufReasoning != "" {
				appendStreamReasoning(&st.messages, st.bufReasoning)
				st.bufReasoning = ""
			}
			if st.bufContent != "" {
				appendStreamContent(&st.messages, st.bufContent)
				st.bufContent = ""
			}
			if hasAnyContent {
				timeoutText := "\n⏱️ AI 响应超时（60s），请检查网络或 API 配置"
				if len(st.messages) > 0 {
					last := st.messages[len(st.messages)-1]
					if last.Role == "assistant" && last.Content == "" {
						timeoutText = "⏱️ AI 响应超时（60s），请检查网络或 API 配置"
					}
				}
				appendStreamContent(&st.messages, timeoutText)
			}
			est := st.promptTokens + st.completionTokens
			st.sessionTotal += est
			st.requestCount++
			if st.requestCount > 1 {
				view.SetTokenText(fmt.Sprintf("⏱ %d · ∑ %d tokens", est, st.sessionTotal))
			} else {
				view.SetTokenText(fmt.Sprintf("⏱ %d tokens", est))
			}
			st.jumpToBottom()
			render()
			continue
		case data, ok := <-inputCh:
			if !ok {
				return nil
			}
			pending = append(pending, data...)
		}

		if st.aiPending {
			closed := false
		aiDrain:
			for {
				select {
				case data, ok := <-inputCh:
					if !ok {
						closed = true
						break aiDrain
					}
					pending = append(pending, data...)
				default:
					break aiDrain
				}
			}
			for {
				// SGR 鼠标序列先交给 handleMouse 取坐标处理（拖选/滚轮）。
				if len(pending) >= 3 && pending[0] == '\x1b' && pending[1] == '[' && pending[2] == '<' {
					mn, dirty := handleMouse()
					if mn == 0 {
						break // 序列不完整，等更多字节
					}
					pending = pending[mn:]
					if dirty {
						render()
					}
					continue
				}
				code, _, n := input.Parse(pending)
				if n == 0 {
					break
				}
				pending = pending[n:]
				// 非 ESC 按键重置「双击 ESC」计时窗口。
				if code != input.KeyEscape {
					st.escPendingAt = time.Time{}
				}
				switch code {
				case input.KeyQuit:
					// AI 回复中：有选区则复制，否则取消回复。
					if !copySelection() {
						cancelAI()
					}
				case input.KeyEscape:
					// 短时间内连按两次 ESC 取消回复。
					now := time.Now()
					if !st.escPendingAt.IsZero() && now.Sub(st.escPendingAt) <= 600*time.Millisecond {
						st.escPendingAt = time.Time{}
						cancelAI()
					} else {
						st.escPendingAt = now
					}
				}
			}
			render()
			if closed {
				return nil
			}
			continue
		}

		closed := false
	drain:
		for {
			select {
			case data, ok := <-inputCh:
				if !ok {
					closed = true
					break drain
				}
				pending = append(pending, data...)
			default:
				break drain
			}
		}

		for {
			// SGR 鼠标序列先交给 handleMouse 取坐标处理（拖选/滚轮）。
			if len(pending) >= 3 && pending[0] == '\x1b' && pending[1] == '[' && pending[2] == '<' {
				mn, dirty := handleMouse()
				if mn == 0 {
					break // 序列不完整，等更多字节
				}
				pending = pending[mn:]
				if dirty {
					render()
				}
				continue
			}
			code, r, n := input.Parse(pending)
			if n == 0 {
				break
			}
			pending = pending[n:]

			if code != input.KeyQuit && st.quitPending {
				st.quitPending = false
				view.SetQuitPending(false)
				if quitTimer != nil {
					quitTimer.Stop()
					quitTimer = nil
					quitTimerCh = nil
				}
			}
			// 取消 AI 后一旦有按键输入，立即恢复正常占位（不必等 3 秒计时器）。
			if abortCh != nil {
				view.SetAborted(false)
				if abortTimer != nil {
					abortTimer.Stop()
				}
				abortTimer = nil
				abortCh = nil
			}
			view.ClearFlash()

			// Ctrl+C：有选区则复制（OSC52），消费掉本次按键不再走退出/清空逻辑。
			if code == input.KeyQuit && copySelection() {
				render()
				continue
			}
			// 其它按键（输入、移动光标等）清空选区高亮。
			if code != input.KeyQuit {
				clearSelection()
			}

			switch code {
			case input.KeyQuit:
				if ed.Len() > 0 {
					ed.Clear()
					break
				}
				if st.quitPending {
					return nil
				}
				st.quitPending = true
				view.SetQuitPending(true)
				quitTimer = time.NewTimer(5 * time.Second)
				quitTimerCh = quitTimer.C
			case input.KeyEnter:
				text := ed.String()
				if text != "" {
					ed.Clear()
					trimmed := strings.TrimSpace(text)
					if handleCommand(trimmed, &st.messages, &st.scroll, view, bg, &bgImages, bgDir, &bgIndex, &bgSlideTicker, &bgSlideCh, &slideReady, &cfg) {
						break
					}
					userMsg := ui.Message{Role: "user", Content: trimmed}
					st.messages = append(st.messages, userMsg)

					// 存一份当前消息快照用于 API 调用
					apiMsgs := make([]ui.Message, len(st.messages))
					copy(apiMsgs, st.messages)

					st.jumpToBottom()
					view.SetSharkActive(true)
					view.SetSharkFrame(0)
					render()

				st.aiPending = true
				st.streamDone = false
				st.escPendingAt = time.Time{}
				// 重置 token 统计
				st.tokenTotal = 0
				st.tokenFinal = false
				st.completionTokens = 0
				total := 0
				for _, m := range apiMsgs {
					total += countTokens(m.Content)
				}
				st.promptTokens = total
				// 预创建一条空的 assistant 消息，后续流式内容会追加至此，
				// 避免 appendStreamContent/appendStreamReasoning 误追加到上轮回复。
				st.messages = append(st.messages, ui.Message{Role: "assistant"})
				// 为本次请求派生可单独取消的子 context，使 Ctrl+C / 双击 ESC
					// 能取消 AI 流，而不影响 readInput / watchResize 常驻协程。
					aiCtx, cancelReq := context.WithCancel(ctx)
					aiCancel = cancelReq
					// 每次请求一条新通道；goroutine 捕获自己这条，避免取消后旧 goroutine
					// 的 close/残留 delta 污染下一轮请求。
					reqCh := make(chan ui.Message, 64)
					streamCh = reqCh
					emitTicker = time.NewTicker(emitInterval)
					emitCh = emitTicker.C
					sharkTicker = time.NewTicker(sharkInterval)
					sharkCh = sharkTicker.C
					aiTimeoutTimer = time.NewTimer(60 * time.Second)
					aiTimeoutCh = aiTimeoutTimer.C
					go func() {
						defer func() {
							if r := recover(); r != nil {
								select {
								case reqCh <- ui.Message{Role: "assistant", Content: fmt.Sprintf("💥 内部错误：%v", r)}:
								case <-aiCtx.Done():
								}
							}
						}()
						streamResponse(aiCtx, provider, apiMsgs, cfg, reqCh)
					}()
				}
			case input.KeyUp:
				ed.HandleKey(code, r)
			case input.KeyDown:
				ed.HandleKey(code, r)
			default:
				if ed.HandleKey(code, r) {
					return nil
				}
			}
		}

		render()

		if closed {
			return nil
		}
	}
}

// appendStreamContent 将流式内容 delta 追加到 messages 中最后一条 assistant 消息。
func appendStreamContent(msgs *[]ui.Message, delta string) {
	for i := len(*msgs) - 1; i >= 0; i-- {
		if (*msgs)[i].Role == "assistant" {
			(*msgs)[i].Content += delta
			return
		}
	}
	// 没有 assistant 消息时追加一条
	*msgs = append(*msgs, ui.Message{Role: "assistant", Content: delta})
}

// appendStreamReasoning 将流式思考过程追加到 messages 中最后一条 assistant 消息的 Reasoning 字段。
func appendStreamReasoning(msgs *[]ui.Message, delta string) {
	for i := len(*msgs) - 1; i >= 0; i-- {
		if (*msgs)[i].Role == "assistant" {
			(*msgs)[i].Reasoning += delta
			return
		}
	}
	*msgs = append(*msgs, ui.Message{Role: "assistant", Reasoning: delta})
}

// storeThinkingDuration 将 thinkingStart 耗时写入最后一条 assistant 消息的 ThinkingDuration。
func storeThinkingDuration(msgs *[]ui.Message, thinkingStart time.Time) {
	if thinkingStart.IsZero() {
		return
	}
	d := time.Since(thinkingStart)
	if d < minThinkingDuration {
		return
	}
	for i := len(*msgs) - 1; i >= 0; i-- {
		if (*msgs)[i].Role == "assistant" {
			(*msgs)[i].ThinkingDuration = d.Round(time.Second)
			return
		}
	}
}

// takeRunes 返回 s 的前 n 个 rune 以及剩余部分（按字节偏移正确切分）。
func takeRunes(s string, n int) (head, tail string) {
	if n <= 0 {
		return "", s
	}
	i := 0
	for ; i < len(s) && n > 0; n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i], s[i:]
}

// streamResponse 以流式方式获取 AI 回复，将每个内容块发送到 ch。
// provider 为 nil 时直接把兜底回复发送到 ch 后关闭 ch。
func streamResponse(ctx context.Context, provider llm.Provider, messages []ui.Message, cfg config.Config, ch chan<- ui.Message) {
	defer close(ch)

	if provider == nil {
		ch <- ui.Message{Role: "assistant", Content: "我是基于StrikeCore的AI智能体助手，请问有什么我可以为你帮助的吗？\n\n  1.我可以帮你整理文档\n\n  2.我可以帮你检查当前设备的运行状态\n\n 你可以试着向我提问。\n\n（提示：配置 data/api.json 并设置 API Key 即可接入真实大模型）"}
		return
	}

	llmMsgs := make([]llm.Message, 0, len(messages))
	for _, m := range messages {
		// 仅 user / assistant 是合法对话角色；其它一律按 user 处理。
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		llmMsgs = append(llmMsgs, llm.Message{Role: role, Content: m.Content})
	}

	opts := &llm.ChatOptions{
		SystemPrompt: cfg.SystemPrompt,
	}
	if cfg.ModelName != "" {
		opts.Model = cfg.ModelName
	}

	streamCh, err := provider.ChatStream(ctx, llmMsgs, opts)
	if err != nil {
		select {
		case ch <- ui.Message{Role: "assistant", Content: fmt.Sprintf("（API 调用失败：%v）", err)}:
		case <-ctx.Done():
		}
		return
	}

	for msg := range streamCh {
		u := ui.Message{Role: "assistant", Content: msg.Content}
		if msg.ReasoningContent != "" {
			u.Reasoning = msg.ReasoningContent
		}
		if msg.Usage != nil {
			u.TokenUsage = &ui.TokenUsage{
				PromptTokens:     msg.Usage.PromptTokens,
				CompletionTokens: msg.Usage.CompletionTokens,
				TotalTokens:      msg.Usage.TotalTokens,
			}
		}
		select {
		case ch <- u:
		case <-ctx.Done():
			return
		}
	}
}

// handleCommand 处理以 / 开头的输入。返回 true 表示已作为命令处理完毕。
func handleCommand(text string, messages *[]ui.Message, msgScroll *int, view *ui.View, bg *ui.Background, bgImages *[]string, bgDir string, bgIndex *int, bgSlideTicker **time.Ticker, bgSlideCh *<-chan time.Time, slideReady *bool, cfg *config.Config) bool {
	if !strings.HasPrefix(text, "/") {
		return false
	}
	parts := strings.Fields(text)
	cmd := parts[0]

	switch cmd {
	case "/reload":
		// 重新读取配置文件、刷新壁纸列表、应用所有设置。
		dirCfg := ui.ReadBgDirCfg(bgDir)

		// 重新扫描图片
		imgs := ui.ListBgImages(bgDir)
		*bgImages = imgs

		// 确定当前壁纸
		if len(imgs) > 0 {
			if dirCfg.Wallpaper != nil && *dirCfg.Wallpaper != "" {
				p := filepath.Join(bgDir, *dirCfg.Wallpaper)
				if _, err := os.Stat(p); err == nil {
					bg.Load(p)
				} else {
					bg.Load(imgs[0])
				}
			} else {
				bg.Load(imgs[0])
			}
			*bgIndex = 0
			for i, p := range imgs {
				if p == bg.Path() {
					*bgIndex = i
					break
				}
			}
		}

		// 透明度
		if dirCfg.BubbleBgOpacity != nil {
			view.SetBubbleBgOpacity(*dirCfg.BubbleBgOpacity)
		}

		// 亮度
		if dirCfg.Brightness != nil {
			cfg.Brightness = *dirCfg.Brightness
			bg.SetBrightness(*dirCfg.Brightness)
		}

		// 幻灯片
		if *bgSlideTicker != nil {
			(*bgSlideTicker).Stop()
			*bgSlideTicker = nil
			*bgSlideCh = nil
		}
		if dirCfg.Enabled != nil && !*dirCfg.Enabled {
			*slideReady = false
		} else {
			*slideReady = true
			if dirCfg.Interval != nil && *dirCfg.Interval > 0 {
				cfg.BgInterval = time.Duration(*dirCfg.Interval) * time.Second
			}
			if cfg.BgInterval > 0 && len(imgs) > 1 {
				*bgSlideTicker = time.NewTicker(cfg.BgInterval)
				*bgSlideCh = (*bgSlideTicker).C
				nextIdx := (*bgIndex + 1) % len(imgs)
				bg.Preload(imgs[nextIdx])
			}
		}

		view.Flash(fmt.Sprintf("配置已刷新（%d 张壁纸）", len(imgs)))

	default:
		*messages = append(*messages, ui.Message{Role: "assistant", Content: fmt.Sprintf("未知命令：%s\n可用命令：/reload   /status", cmd)})
		*msgScroll = scrollToBottom
	}
	return true
}

func enterSession(out io.Writer) {
	io.WriteString(out, enterAltScreen)
	io.WriteString(out, hideCursor)
	io.WriteString(out, enableMouse)
	io.WriteString(out, clearScreen)
	io.WriteString(out, cursorHome)
}

func leaveSession(out io.Writer) {
	io.WriteString(out, disableMouse)
	io.WriteString(out, showCursor)
	io.WriteString(out, leaveAltScreen)
}

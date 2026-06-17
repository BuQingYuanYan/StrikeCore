// Package app 将终端、屏幕和视图组合在一起并运行事件循环。
// 它管理进程级别的问题：原始模式设置/恢复、恐慌恢复、信号处理以及通过上下文管理协程生命周期。
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

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
	// 鼠标上报：?1000h 开启按键事件上报，?1006h 启用 SGR 扩展坐标编码
	//（坐标无 223 上限，且滚轮/释放可区分）。离开会话时成对关闭。
	enableMouse  = "\x1b[?1000h\x1b[?1006h"
	disableMouse = "\x1b[?1006l\x1b[?1000l"
)

// scrollStep 是一次滚轮事件滚动的行数。
const scrollStep = 3

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
	bg := ui.NewBackground(cfg.BgPath, cfg.Brightness)
	view := ui.NewView(s, cfg, bg, workDir)
	if bgDirCfg.BubbleBgOpacity != nil {
		view.SetBubbleBgOpacity(*bgDirCfg.BubbleBgOpacity)
	}
	ed := &editor.Editor{}
	var (
		messages  []ui.Message
		msgScroll int
	)

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
		view.SetMessages(messages, msgScroll)
		cur := view.Render(ed, w, h)
		// Render 会把 msgScroll 钳制到合法范围（尤其是发送消息后设的
		// “滚动到底部”哨兵值）；读回钳制结果，使后续滚轮上滚从真实底部开始。
		msgScroll = view.ScrollOffset()
		s.Flush(cur)
	}

	render()

	// 壁纸幻灯片放映——定期切换到下一张图像。
	// 下一张图像在后台协程中预先解码，因此实际切换是即时的，在调整大小期间不会卡顿。
	var (
		bgSlideTicker *time.Ticker
		bgSlideCh     <-chan time.Time
		bgIndex       int
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
	}()

	inputCh := make(chan []byte, 128)
	resizeCh := make(chan struct{}, 1)
	go readInput(ctx, term, inputCh, resizeCh)
	go watchResize(ctx, term, resizeCh)

	var (
		pending       []byte
		debounceTimer *time.Timer
		debounceCh    <-chan time.Time
		quitPending   bool
		quitTimer     *time.Timer
		quitTimerCh   <-chan time.Time
	)
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		if quitTimer != nil {
			quitTimer.Stop()
		}
	}()

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
			bgIndex = (bgIndex + 1) % len(bgImages)
			if !bg.Activate(bgImages[bgIndex]) {
				bg.Load(bgImages[bgIndex]) // fallback: synchronous
			}
			// 启动下一张之后的图像的异步预解码。
			nextIdx := (bgIndex + 1) % len(bgImages)
			bg.Preload(bgImages[nextIdx])
			render()
			continue
		case <-quitTimerCh:
			quitPending = false
			view.SetQuitPending(false)
			quitTimer = nil
			quitTimerCh = nil
			render()
			continue
		case data, ok := <-inputCh:
			if !ok {
				return nil // 标准输入已关闭
			}
			pending = append(pending, data...)
		}

		// 排空任何进一步的缓冲输入而不阻塞。检查 ok，以便关闭的通道会中断循环，而不是在其始终就绪的零值上旋转。
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
			code, r, n := input.Parse(pending)
			if n == 0 {
				break
			}
			pending = pending[n:]

			if code != input.KeyQuit && quitPending {
				quitPending = false
				view.SetQuitPending(false)
				if quitTimer != nil {
					quitTimer.Stop()
					quitTimer = nil
					quitTimerCh = nil
				}
			}
			view.ClearFlash()

			switch code {
			case input.KeyQuit:
				if ed.Len() > 0 {
					ed.Clear()
					break // 编辑器有字则清空，不触发退出确认
				}
				if quitPending {
					return nil
				}
				quitPending = true
				view.SetQuitPending(true)
				quitTimer = time.NewTimer(5 * time.Second)
				quitTimerCh = quitTimer.C
			case input.KeyEnter:
				text := ed.String()
				if text != "" {
					ed.Clear()
					if handleCommand(text, &messages, &msgScroll, view, bg, &bgImages, bgDir, &bgIndex, &bgSlideTicker, &bgSlideCh, &slideReady, &cfg) {
						break
					}
					userMsg := ui.Message{Role: "user", Content: text}
					messages = append(messages, userMsg)
					reply := getAIResponse(ctx, provider, messages, cfg)
					messages = append(messages, ui.Message{Role: "assistant", Content: reply})
					msgScroll = 999999 // 自动滚动到底部，在 drawMessages 中限制
				}
		case input.KeyUp:
			ed.HandleKey(code, r)
		case input.KeyDown:
			ed.HandleKey(code, r)
			case input.KeyScrollUp:
				// 滚轮始终滚动消息，与编辑器是否有内容无关。
				if msgScroll > 0 {
					msgScroll -= scrollStep
					if msgScroll < 0 {
						msgScroll = 0
					}
				}
			case input.KeyScrollDown:
				if msgScroll < 999999 {
					msgScroll += scrollStep
					if msgScroll > 999999 {
						msgScroll = 999999
					}
				}
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

// getAIResponse 调用 LLM 获取回复。provider 为 nil 时返回内置兜底回复。
func getAIResponse(ctx context.Context, provider llm.Provider, messages []ui.Message, cfg config.Config) string {
	if provider == nil {
		return "我是基于StrikeCore的AI智能体助手，请问有什么我可以为你帮助的吗？\n\n  1.我可以帮你整理文档\n\n  2.我可以帮你检查当前设备的运行状态\n\n 你可以试着向我提问。\n\n（提示：配置 data/api.json 并设置 API Key 即可接入真实大模型）"
	}

	llmMsgs := make([]llm.Message, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "assistant"
		} else {
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

	reply, err := provider.Chat(ctx, llmMsgs, opts)
	if err != nil {
		return fmt.Sprintf("（API 调用失败：%v）", err)
	}
	return reply
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
		*messages = append(*messages, ui.Message{Role: "assistant", Content: fmt.Sprintf("未知命令：%s\n可用命令：/reload", cmd)})
		*msgScroll = 999999
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

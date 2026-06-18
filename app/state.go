package app

import (
	"time"
	"unicode/utf8"

	"strike-core/internal/ui"
)

// emitCount 根据待输出缓冲的积压量，决定本次 tick 吐出多少个 rune。
// 积压越多吐越快（backlog/emitDivisor），但不少于 emitMinRunes、不多于 emitMaxRunes，
// 也不会超过实际剩余字符数。这样模型返回快时 UI 迅速追平、不积压成延迟，
// 返回慢时仍保留逐字打字机手感。
func emitCount(buf string) int {
	backlog := utf8.RuneCountInString(buf)
	if backlog == 0 {
		return 0
	}
	n := backlog / emitDivisor
	n = max(n, emitMinRunes)
	n = min(n, emitMaxRunes)
	n = min(n, backlog)
	return n
}

// chatState 收敛事件循环的纯数据状态：对话历史、滚动位置、流式拼接缓冲、
// 退出确认标志与动画帧计数。它不持有任何 I/O 句柄（终端、计时器、通道），
// 因此可以在不启动真实终端的情况下被单元测试驱动。
//
// I/O 句柄（计时器、通道、view、background）仍由 Run 直接持有；
// 后续阶段会把“状态转移 → 副作用”进一步下沉到这里。
type chatState struct {
	messages []ui.Message
	scroll   int // 流顶部裁掉的行数；scrollToBottom 表示“滚到底部”哨兵

	bufContent   string // 待逐字输出的正文缓冲
	bufReasoning string // 待逐字输出的思考过程缓冲
	aiPending    bool   // AI 正在生成（鲨鱼动画 + 打字机进行中）
	streamDone   bool   // 上游流已结束，缓冲清空后即可收尾

	quitPending bool // 已按一次 Ctrl+C，等待第二次确认退出

	escPendingAt time.Time // AI 回复中上一次按 ESC 的时刻；短时间内再次按 ESC 即取消回复

	sel       ui.Selection // 鼠标自绘选区，锚定流内绝对位置（随滚动跟随内容）
	selecting bool         // 正在拖拽选区（鼠标左键按住中）
}

// scrollUp 向上滚动 n 行，不越过顶部。返回是否发生了变化。
func (c *chatState) scrollUp(n int) bool {
	if c.scroll <= 0 {
		return false
	}
	c.scroll -= n
	if c.scroll < 0 {
		c.scroll = 0
	}
	return true
}

// scrollDown 向下滚动 n 行。实际下界由 View.Render 钳制；这里只防止越过
// 哨兵值导致整数溢出。返回是否发生了变化。
func (c *chatState) scrollDown(n int) bool {
	if c.scroll >= scrollToBottom-n {
		return false
	}
	c.scroll += n
	return true
}

// jumpToBottom 把滚动位置设为底部哨兵，下次 Render 会钳制到真实底部。
func (c *chatState) jumpToBottom() {
	c.scroll = scrollToBottom
}

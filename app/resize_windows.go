//go:build windows

package app

import (
	"context"
	"os"
	"time"

	"strike-core/internal/terminal"
)

// interruptSignals 返回应触发干净关闭的信号。Windows 在通常意义上缺少 SIGTERM，因此我们仅监视 Interrupt。
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// watchResize 在 Windows 上轮询终端大小（没有 SIGWINCH）并在变化时通知 resizeCh。轮询以约 30 Hz 的频率运行，作为 readInput 中基于 ReadConsoleInput 的事件传递的回退。GetConsoleScreenBufferInfo 足够廉价，因此空闲成本不是问题。
func watchResize(ctx context.Context, term terminal.Terminal, resizeCh chan<- struct{}) {
	prevW, prevH, _ := term.Size()
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w, h, err := term.Size()
			if err != nil {
				continue
			}
			if w != prevW || h != prevH {
				prevW, prevH = w, h
				select {
				case resizeCh <- struct{}{}:
				default:
				}
			}
		}
	}
}

//go:build !windows

package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"strike-core/internal/terminal"
)

// interruptSignals 返回应触发干净关闭的信号。
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// watchResize 在 Unix 上使用 SIGWINCH 而不是轮询：内核在窗口大小变化时通知我们，因此空闲成本为零。
func watchResize(ctx context.Context, _ terminal.Terminal, resizeCh chan<- struct{}) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-winch:
			select {
			case resizeCh <- struct{}{}:
			default:
			}
		}
	}
}

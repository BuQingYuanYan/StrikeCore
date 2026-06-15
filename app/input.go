//go:build !windows

package app

import (
	"context"

	"strike-core/internal/terminal"
)

// readInput 将标准输入字节发送到 inputCh。在 Unix 上，调整大小事件完全由 watchResize (SIGWINCH) 处理，因此此函数忽略 resizeCh。
func readInput(ctx context.Context, term terminal.Terminal, inputCh chan<- []byte, _ chan<- struct{}) {
	defer close(inputCh)
	buf := make([]byte, 64)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := term.In().Read(buf)
		if n > 0 {
			cp := make([]byte, n)
			copy(cp, buf[:n])
			select {
			case inputCh <- cp:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

//go:build windows

package app

import (
	"context"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"strike-core/internal/terminal"
)

// Windows 虚拟键码。
const (
	vkBack   = 0x08
	vkTab    = 0x09
	vkClear  = 0x0C
	vkReturn = 0x0D
	vkShift  = 0x10
	vkCtrl   = 0x11
	vkMenu   = 0x12
	vkPause  = 0x13
	vkEscape = 0x1B

	vkPrior = 0x21 // 向上翻页
	vkNext  = 0x22 // 向下翻页
	vkEnd   = 0x23
	vkHome  = 0x24
	vkLeft  = 0x25
	vkUp    = 0x26
	vkRight = 0x27
	vkDown  = 0x28

	vkInsert = 0x2D
	vkDelete = 0x2E

	vkF1  = 0x70
	vkF2  = 0x71
	vkF3  = 0x72
	vkF4  = 0x73
	vkF5  = 0x74
	vkF6  = 0x75
	vkF7  = 0x76
	vkF8  = 0x77
	vkF9  = 0x78
	vkF10 = 0x79
	vkF11 = 0x7A
	vkF12 = 0x7B
)

var procReadConsoleInput = syscall.NewLazyDLL("kernel32.dll").NewProc("ReadConsoleInputW")

// readInput 在 Windows 上使用 ReadConsoleInput 读取控制台输入事件。
// KEY_EVENT 记录转换为字节序列并发送到 inputCh。
// WINDOW_BUFFER_SIZE_EVENT 记录转发到 resizeCh。
// 当标准输入被重定向（不是真正的控制台）时，ReadConsoleInput 失败，将改用回退字节读取器。
func readInput(ctx context.Context, term terminal.Terminal, inputCh chan<- []byte, resizeCh chan<- struct{}) {
	defer close(inputCh)

	// 获取控制台输入句柄。如果终端未暴露句柄（例如标准输入被重定向），则回退到通用字节读取。
	type handleProvider interface {
		ConsoleInputHandle() syscall.Handle
	}
	hp, ok := term.(handleProvider)
	if !ok {
		readInputFallback(ctx, term, inputCh)
		return
	}
	h := hp.ConsoleInputHandle()

	var rec terminal.InputRecord
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var readCount uint32
		r, _, _ := procReadConsoleInput.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&rec)),
			1,
			uintptr(unsafe.Pointer(&readCount)),
		)
		if r == 0 {
			// ReadConsoleInput 失败（例如标准输入是管道）；回退。
			readInputFallback(ctx, term, inputCh)
			return
		}
		if readCount == 0 {
			continue
		}

		switch rec.EventType {
		case terminal.WindowBufferSizeEvent:
			select {
			case resizeCh <- struct{}{}:
			default:
			}

		case terminal.KeyEvent:
			ke := rec.KeyEvent
			if ke.KeyDown == 0 {
				continue // 键释放
			}

			buf := keyEventToBytes(ke)
			if len(buf) == 0 {
				continue
			}
			select {
			case inputCh <- buf:
			case <-ctx.Done():
				return
			}

		case terminal.MouseEvent:
			me := rec.AsMouseEvent()
			if me.EventFlags&terminal.MouseWheeled == 0 {
				continue // 只关心垂直滚轮
			}
			// 滚轮增量在 ButtonState 的高 16 位，按有符号处理：
			// 正=向上滚，负=向下滚。转成解析器识别的 SGR 字节序列。
			delta := int16(me.ButtonState >> 16)
			var seq []byte
			if delta > 0 {
				seq = []byte("\x1b[<64;1;1M")
			} else {
				seq = []byte("\x1b[<65;1;1M")
			}
			select {
			case inputCh <- seq:
			case <-ctx.Done():
				return
			}
		}
	}
}

// readInputFallback 是当 ReadConsoleInput 不可用（标准输入是管道或重定向文件）时使用的通用字节读取器。
// 它不会关闭 inputCh——调用者 (readInput) 拥有通道的生命周期。
func readInputFallback(ctx context.Context, term terminal.Terminal, inputCh chan<- []byte) {
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

// keyEventToBytes 将 KEY_EVENT_RECORD 转换为与控制台在原始+VT 输入模式下产生的字节序列匹配的字节序列。
func keyEventToBytes(ke terminal.KeyEventRecord) []byte {
	// 特殊键产生 VT 转义序列。
	if seq := specialKeySeq(ke); seq != nil {
		return seq
	}

	// 来自 UnicodeChar 的常规字符。
	ch := ke.UnicodeChar
	if ch == 0 {
		return nil
	}

	ctrl := ke.ControlKeyState
	alt := ctrl&terminal.LeftAltPressed != 0 || ctrl&terminal.RightAltPressed != 0

	switch {
	case ch == '\r' || ch == '\n':
		return []byte{'\r'}
	case ch == '\t':
		return []byte{'\t'}
	case ch == '\b':
		return []byte{0x7f}
	case ch == 0x1b:
		return []byte{0x1b}
	}

	// Ctrl+字母产生控制字符 (0x01-0x1a)。
	if ctrl&(terminal.LeftCtrlPressed|terminal.RightCtrlPressed) != 0 && ch >= 'a' && ch <= 'z' {
		return []byte{byte(ch - 'a' + 1)}
	}
	if ctrl&(terminal.LeftCtrlPressed|terminal.RightCtrlPressed) != 0 && ch >= 'A' && ch <= 'Z' {
		return []byte{byte(ch - 'A' + 1)}
	}

	// Alt+字符：添加 ESC 前缀。
	if alt {
		r := utf8.EncodeRune(buf8[:], rune(ch))
		out := make([]byte, 0, r+1)
		out = append(out, 0x1b)
		out = append(out, buf8[:r]...)
		return out
	}

	// 纯 UTF-8 字符。
	r := utf8.EncodeRune(buf8[:], rune(ch))
	cp := make([]byte, r)
	copy(cp, buf8[:r])
	return cp
}

var buf8 [utf8.UTFMax]byte

// specialKeySeq 将虚拟键码映射到 VT 转义序列。
func specialKeySeq(ke terminal.KeyEventRecord) []byte {
	ctrl := ke.ControlKeyState
	alt := ctrl&terminal.LeftAltPressed != 0 || ctrl&terminal.RightAltPressed != 0
	isCtrl := ctrl&(terminal.LeftCtrlPressed|terminal.RightCtrlPressed) != 0

	var seq string
	switch ke.VirtualKeyCode {
	case vkUp:
		seq = "\x1b[A"
	case vkDown:
		seq = "\x1b[B"
	case vkRight:
		seq = "\x1b[C"
	case vkLeft:
		seq = "\x1b[D"
	case vkHome:
		seq = "\x1b[H"
	case vkEnd:
		seq = "\x1b[F"
	case vkPrior:
		seq = "\x1b[5~"
	case vkNext:
		seq = "\x1b[6~"
	case vkInsert:
		seq = "\x1b[2~"
	case vkDelete:
		seq = "\x1b[3~"
	case vkEscape:
		seq = "\x1b"
	case vkF1:
		seq = "\x1bOP"
	case vkF2:
		seq = "\x1bOQ"
	case vkF3:
		seq = "\x1bOR"
	case vkF4:
		seq = "\x1bOS"
	case vkF5:
		seq = "\x1b[15~"
	case vkF6:
		seq = "\x1b[17~"
	case vkF7:
		seq = "\x1b[18~"
	case vkF8:
		seq = "\x1b[19~"
	case vkF9:
		seq = "\x1b[20~"
	case vkF10:
		seq = "\x1b[21~"
	case vkF11:
		seq = "\x1b[23~"
	case vkF12:
		seq = "\x1b[24~"
	default:
		return nil
	}

	// Ctrl+箭头在某些终端中产生不同的序列。
	if isCtrl {
		switch ke.VirtualKeyCode {
		case vkLeft:
			seq = "\x1b[1;5D"
		case vkRight:
			seq = "\x1b[1;5C"
		case vkUp:
			seq = "\x1b[1;5A"
		case vkDown:
			seq = "\x1b[1;5B"
		}
	}

	if alt {
		return append([]byte{0x1b}, []byte(seq)...)
	}
	return []byte(seq)
}

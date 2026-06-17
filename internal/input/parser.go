package input

import "unicode/utf8"

// Parse 从字节片中解码第一个按键（可能包含部分或多键转义序列）。它返回按键码、rune（对于 KeyRune）以及消耗的字节数。n == 0 表示缓冲区尚未包含完整的、可操作的按键——调用者应等待更多字节。
//
// Parse 是纯函数：它没有副作用，可完全通过表格驱动测试。
func Parse(b []byte) (code int, r rune, n int) {
	if len(b) == 0 {
		return KeyNone, 0, 0
	}

	if b[0] == '\x1b' {
		return parseEscape(b)
	}

	switch {
	case b[0] == '\x7f' || b[0] == '\x08':
		return KeyBackspace, 0, 1
	case b[0] == '\x03':
		return KeyQuit, 0, 1
	case b[0] == '\r' || b[0] == '\n':
		return KeyEnter, 0, 1
	case b[0] < 0x20:
		return KeyNone, 0, 0
	}

	dr, size := utf8.DecodeRune(b)
	if dr != utf8.RuneError || size > 1 {
		return KeyRune, dr, size
	}
	// 尾部不完整的 UTF-8 多字节序列：等待更多字节。
	return KeyNone, 0, 0
}

func parseEscape(b []byte) (int, rune, int) {
	if len(b) < 2 {
		return KeyEscape, 0, 1
	}
	switch b[1] {
	case '[':
		return parseCSI(b)
	case 'O':
		if len(b) >= 3 {
			switch b[2] {
			case 'H':
				return KeyHome, 0, 3
			case 'F':
				return KeyEnd, 0, 3
			}
		}
	}
	return KeyEscape, 0, 1
}

func parseCSI(b []byte) (int, rune, int) {
	if len(b) < 3 {
		return KeyEscape, 0, 1
	}
	if b[2] == '<' {
		return parseSGRMouse(b)
	}
	switch b[2] {
	case 'A':
		return KeyUp, 0, 3
	case 'B':
		return KeyDown, 0, 3
	case 'C':
		return KeyRight, 0, 3
	case 'D':
		return KeyLeft, 0, 3
	case 'H':
		return KeyHome, 0, 3
	case 'F':
		return KeyEnd, 0, 3
	case '1':
		if len(b) >= 4 && b[3] == '~' {
			return KeyHome, 0, 4
		}
	case '3':
		if len(b) >= 4 && b[3] == '~' {
			return KeyDelete, 0, 4
		}
	case '4':
		if len(b) >= 4 && b[3] == '~' {
			return KeyEnd, 0, 4
		}
	}
	return KeyEscape, 0, 1
}

// parseSGRMouse 解析 SGR 扩展鼠标序列 \x1b[<Cb;Cx;Cy(M|m)。
// 仅关心滚轮：Cb=64 为上滚、Cb=65 为下滚。其余鼠标事件（点击/移动/拖拽）
// 被完整消耗但映射为 KeyNone，以免污染输入流。
// 序列不完整时返回 n==0，等待更多字节。
func parseSGRMouse(b []byte) (int, rune, int) {
	// 从 b[3] 起读取首个数字参数 Cb，直到遇到 ';' 或终结符。
	cb := 0
	haveCb := false
	i := 3
	for ; i < len(b); i++ {
		c := b[i]
		if c >= '0' && c <= '9' {
			cb = cb*10 + int(c-'0')
			haveCb = true
		} else {
			break
		}
	}
	if !haveCb {
		// 畸形序列，没有参数：丢弃 ESC 以免卡住。
		return KeyNone, 0, 1
	}
	// 扫描终结符以确定序列长度；仅 'M'（按下）产生滚轮事件，
	// 'm'（释放）被消耗并丢弃以防某些终端同时发送两次导致重复滚动。
	for ; i < len(b); i++ {
		if b[i] == 'M' {
			n := i + 1
			switch cb {
			case 64:
				return KeyScrollUp, 0, n
			case 65:
				return KeyScrollDown, 0, n
			default:
				return KeyNone, 0, n
			}
		}
		if b[i] == 'm' {
			return KeyNone, 0, i+1
		}
	}
	// 终结符尚未到达：等待更多字节。
	return KeyNone, 0, 0
}

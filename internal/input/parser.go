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

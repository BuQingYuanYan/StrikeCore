package input

// MouseKind 标识 SGR 鼠标事件类别。
type MouseKind int

const (
	MouseOther     MouseKind = iota
	MousePress
	MouseDrag
	MouseRelease
	MouseWheelUp
	MouseWheelDown
)

// MouseEvent 表示一次鼠标事件，坐标为 0-based 单元格（上报为 1-based，已减 1）。
type MouseEvent struct {
	X, Y int
	Kind MouseKind
}

// ParseSGRMouse 解析 \x1b[<Cb;Cx;Cy(M|m) 序列，返回事件、成功标志和消耗字节数。
//
//   - ok=false, n==0: 序列不完整，需更多字节。
//   - ok=false, n>0:  畸形序列，已跳过 n 字节。
//   - ok=true:         成功，n 为整段序列长度。
func ParseSGRMouse(b []byte) (ev MouseEvent, ok bool, n int) {
	// 必须以 \x1b[< 开头。
	if len(b) < 3 || b[0] != '\x1b' || b[1] != '[' || b[2] != '<' {
		return MouseEvent{}, false, 0
	}

	i := 3
	cb, okCb, ni := scanUint(b, i)
	if ni >= len(b) {
		return MouseEvent{}, false, 0 // 不完整
	}
	if !okCb || b[ni] != ';' {
		return MouseEvent{}, false, 1 // 畸形：丢弃 ESC
	}
	i = ni + 1

	cx, okCx, ni := scanUint(b, i)
	if ni >= len(b) {
		return MouseEvent{}, false, 0
	}
	if !okCx || b[ni] != ';' {
		return MouseEvent{}, false, 1
	}
	i = ni + 1

	cy, okCy, ni := scanUint(b, i)
	if ni >= len(b) {
		return MouseEvent{}, false, 0
	}
	if !okCy || (b[ni] != 'M' && b[ni] != 'm') {
		return MouseEvent{}, false, 1
	}
	final := b[ni]
	n = ni + 1

	ev = MouseEvent{
		X:    cx - 1, // 终端 1-based -> 0-based
		Y:    cy - 1,
		Kind: classify(cb, final),
	}
	return ev, true, n
}

// classify 根据按钮码 Cb 和终结符判定事件类别。
func classify(cb int, final byte) MouseKind {
	// 滚轮：bit6(64)=上，bit6+bit0(65)=下。
	if cb&64 != 0 {
		if final != 'M' {
			return MouseOther
		}
		switch cb & 0x43 { // 低 2 位区分上/下
		case 64:
			return MouseWheelUp
		case 65:
			return MouseWheelDown
		default:
			return MouseOther
		}
	}
	if final == 'm' {
		return MouseRelease
	}
	if cb&32 != 0 { // bit5=拖拽
		if cb&0x03 == 0 { // 仅左键
			return MouseDrag
		}
		return MouseOther
	}
	if cb&0x03 == 0 { // 左键按下
		return MousePress
	}
	return MouseOther
}

// scanUint 从 b[i] 读取十进制数字，返回数值和停止位置。
func scanUint(b []byte, i int) (val int, ok bool, next int) {
	start := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		val = val*10 + int(b[i]-'0')
		i++
	}
	return val, i > start, i
}

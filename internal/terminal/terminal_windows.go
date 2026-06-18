//go:build windows

package terminal

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"
)

const (
	enableVirtualTerminalProcessing = 0x0004
	enableWindowInput               = 0x0008
	enableMouseInput                = 0x0010
	enableEchoInput                 = 0x0004
	enableLineInput                 = 0x0002
	enableProcessedInput            = 0x0001
	enableQuickEditMode             = 0x0040
	enableExtendedFlags             = 0x0080
)

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode         = kernel32.NewProc("SetConsoleMode")
	procGetConsoleMode         = kernel32.NewProc("GetConsoleMode")
	procGetConsoleScreenBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct {
	x, y int16
}
type smallRect struct {
	left, top, right, bottom int16
}
type consoleScreenBufferInfo struct {
	size       coord
	cursor     coord
	attributes uint16
	window     smallRect
	maxSize    coord
}

type windowsTerminal struct {
	in        *os.File
	out       *os.File
	inFd      syscall.Handle
	outFd     syscall.Handle
	conout    syscall.Handle // separate CONOUT$ handle for size + VT enable
	hasConout bool
}

// ─── Windows 控制台输入事件类型 ─────────────────────────────────────────────

const (
	KeyEvent              = 0x0001
	MouseEvent            = 0x0002
	WindowBufferSizeEvent = 0x0004
)

// KeyEventRecord 映射 Windows KEY_EVENT_RECORD 结构体。
type KeyEventRecord struct {
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

// MouseEventRecord 映射 Windows MOUSE_EVENT_RECORD 结构体。它与
// KeyEventRecord 共享 INPUT_RECORD 联合体（两者均为 16 字节）。
type MouseEventRecord struct {
	MousePosition   coord
	ButtonState     uint32
	ControlKeyState uint32
	EventFlags      uint32
}

// 鼠标事件标志位（dwEventFlags）与按键状态（dwButtonState）。
const (
	MouseMoved   = 0x0001 // dwEventFlags：鼠标移动
	MouseWheeled = 0x0004 // dwEventFlags：垂直滚轮事件

	FromLeft1stButtonPressed = 0x0001 // dwButtonState：左键
)

// InputRecord 映射 Windows INPUT_RECORD 结构体（EventType + 联合体）。
type InputRecord struct {
	EventType uint16
	_         [2]byte // padding to align the union to 4 bytes
	KeyEvent  KeyEventRecord
}

// AsMouseEvent 把联合体重新解释为鼠标事件记录。仅当
// EventType == MouseEvent 时调用方可信赖其内容。
func (r *InputRecord) AsMouseEvent() MouseEventRecord {
	return *(*MouseEventRecord)(unsafe.Pointer(&r.KeyEvent))
}

// Pos 返回鼠标的 0-based 单元格坐标（coord 字段未导出，供其它包读取）。
func (m MouseEventRecord) Pos() (x, y int) {
	return int(m.MousePosition.x), int(m.MousePosition.y)
}

// dwControlKeyState 标志位。
const (
	RightAltPressed  = 0x0001
	LeftAltPressed   = 0x0002
	RightCtrlPressed = 0x0004
	LeftCtrlPressed  = 0x0008
	ShiftPressed     = 0x0010
)

// ConsoleInputHandle 返回控制台输入缓冲区的原生句柄。
// 由 Windows 输入读取器用于直接调用 ReadConsoleInputW。
func (t *windowsTerminal) ConsoleInputHandle() syscall.Handle {
	return t.inFd
}

// New 构造 Windows 终端后端，在构造时（而非包初始化时）解析句柄，
// 以便正确处理重定向的流。
func New() (Terminal, error) {
	t := &windowsTerminal{
		in:    os.Stdin,
		out:   os.Stdout,
		inFd:  syscall.Handle(os.Stdin.Fd()),
		outFd: syscall.Handle(os.Stdout.Fd()),
	}
	h, err := openConout()
	if err == nil {
		t.conout = h
		t.hasConout = true
	}
	return t, nil
}

func openConout() (syscall.Handle, error) {
	name, err := syscall.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return 0, err
	}
	return h, nil
}

func (t *windowsTerminal) Out() io.Writer { return t.out }
func (t *windowsTerminal) In() io.Reader  { return t.in }

func (t *windowsTerminal) Init() (func(), error) {
	// 在我们拥有的输出句柄上启用 VT 处理。这是尽力而为的：
	// 当 stdout 是真实控制台时成功；当输出重定向到管道时无害地失败
	//（反正那里不会渲染任何内容），保持原有的容错行为而非中止启动。
	_ = enableVT(t.outFd)
	if t.hasConout {
		_ = enableVT(t.conout)
	}

	// 将 stdin 切换到原始模式，记住之前的模式以便恢复。同样
	// 是尽力而为：管道化的 stdin 没有控制台模式可读，但字节读取
	// 仍然有效，因此我们继续执行而不报错。
	prevMode, haveMode := uint32(0), false
	if m, err := getConsoleMode(t.inFd); err == nil {
		prevMode, haveMode = m, true
		// 开启窗口与鼠标事件上报；关闭行编辑/回显/快速编辑。
		// QuickEdit 必须关闭，否则控制台会把鼠标拖拽吞掉用于选区，
		// 鼠标事件无法送达 ReadConsoleInput；关闭它需要同时设置
		// ExtendedFlags 位才能生效。
		rawMode := (m &^ (enableEchoInput | enableLineInput | enableProcessedInput | enableQuickEditMode)) |
			enableWindowInput | enableMouseInput | enableExtendedFlags
		_ = setConsoleMode(t.inFd, rawMode)
	}

	restore := func() {
		if haveMode {
			_ = setConsoleMode(t.inFd, prevMode)
		}
		if t.hasConout {
			syscall.CloseHandle(t.conout)
			t.hasConout = false
		}
	}
	return restore, nil
}

// WindowOrigin 返回可视窗口在控制台屏幕缓冲区中的左上角原点（left, top）。
// 鼠标事件上报的是缓冲区坐标，需减去此原点才能换算成可视区行列。
// 读取失败时返回 (0,0)。
func (t *windowsTerminal) WindowOrigin() (left, top int) {
	fd := t.outFd
	if t.hasConout {
		fd = t.conout
	}
	var csbi consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBuffer.Call(uintptr(fd), uintptr(unsafe.Pointer(&csbi)))
	if r == 0 {
		return 0, 0
	}
	return int(csbi.window.left), int(csbi.window.top)
}

func (t *windowsTerminal) Size() (int, int, error) {
	fd := t.outFd
	if t.hasConout {
		fd = t.conout
	}
	var csbi consoleScreenBufferInfo
	r, _, e := procGetConsoleScreenBuffer.Call(uintptr(fd), uintptr(unsafe.Pointer(&csbi)))
	if r == 0 {
		return 0, 0, fmt.Errorf("terminal: GetConsoleScreenBufferInfo: %w", e)
	}
	w := int(csbi.window.right - csbi.window.left + 1)
	h := int(csbi.window.bottom - csbi.window.top + 1)
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 25
	}
	return w, h, nil
}

func enableVT(fd syscall.Handle) error {
	mode, err := getConsoleMode(fd)
	if err != nil {
		return err
	}
	return setConsoleMode(fd, mode|enableVirtualTerminalProcessing)
}

func getConsoleMode(fd syscall.Handle) (uint32, error) {
	var mode uint32
	r, _, e := procGetConsoleMode.Call(uintptr(fd), uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return 0, e
	}
	return mode, nil
}

func setConsoleMode(fd syscall.Handle, mode uint32) error {
	r, _, e := procSetConsoleMode.Call(uintptr(fd), uintptr(mode))
	if r == 0 {
		return e
	}
	return nil
}

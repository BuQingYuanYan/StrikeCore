// Package terminal 将操作系统相关的终端操作抽象在一个小接口之后，
// 使得应用的其余部分与平台无关。后端通过编译标签
// （terminal_windows.go / terminal_unix.go）在编译时选择。
//
// 所有 VT 转义序列（备屏、光标可见性、同步输出、真彩色）都是可移植的，
// 位于 screen/ui 层——它们不是后端关心的。后端仅处理原始模式、
// 尺寸查询和提供输出写入器。
package terminal

import "io"

// Terminal 是应用所依赖的操作系统相关终端抽象层。
type Terminal interface {
	// Init 将终端切换到原始模式（并在 Windows 上启用 VT 处理）。
	// 它返回一个 restore 函数，该函数必须被调用以恢复终端到之前的状态，
	// 并且可以在 defer/recover 中安全调用。
	Init() (restore func(), err error)

	// Size 报告当前终端的列数和行数尺寸。
	Size() (cols, rows int, err error)

	// Out 是渲染器刷新帧时使用的写入器。
	Out() io.Writer

	// In 是输入循环读取字节时使用的读取器。
	In() io.Reader
}

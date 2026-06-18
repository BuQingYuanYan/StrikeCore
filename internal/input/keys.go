// Package input 定义按键模型和纯终端输入解析器。它是叶子包；编辑器和应用依赖它获取按键码。
package input

// Parse 生成的按键码。零值 KeyNone 表示"无按键"。
const (
	KeyNone = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyPgUp
	KeyPgDown
	KeyEnter
	KeyTab
	KeyEscape
	KeyQuit
	KeyRune
	KeyScrollUp   // 鼠标滚轮向上
	KeyScrollDown // 鼠标滚轮向下
	KeyMouse      // 鼠标点击/拖拽/释放（坐标由 ParseSGRMouse 取得）
)

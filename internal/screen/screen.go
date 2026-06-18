// Package screen 是一个与后端无关的单元格缓冲区，支持基于差异的刷新。它不拥有终端状态，将渲染输出写入 io.Writer，使得渲染器可以针对 bytes.Buffer 进行完整的单元测试。
package screen

import (
	"io"
	"strings"

	"github.com/mattn/go-runewidth"

	"strike-core/internal/style"
)

// Cell 是一个渲染后的网格单元格。W 是显示宽度：普通字符为 1，宽字符（CJK）为 2，-1 表示宽字符的右半部分。
type Cell struct {
	Ch     rune
	Fg, Bg style.Color
	W      int
}

// Screen 保存前后单元格缓冲区和差异渲染所需的光标状态。使用 New 构造，然后使用 Realloc 调整大小。
type Screen struct {
	out io.Writer

	cells     []Cell
	cellsPrev []Cell
	cols      int
	rows      int

	prevCursorVisible bool

	buf    strings.Builder
	intBuf [8]byte
}

// New 返回一个渲染到 out 的 Screen。
func New(out io.Writer) *Screen {
	return &Screen{out: out, prevCursorVisible: true}
}

// Cols 和 Rows 返回当前缓冲区的尺寸。
func (s *Screen) Cols() int { return s.cols }
func (s *Screen) Rows() int { return s.rows }

// Realloc 将两个缓冲区都调整为 c×r 个单元格。现有内容将被丢弃。
func (s *Screen) Realloc(c, r int) {
	n := c * r
	if n == 0 {
		n = 1
	}
	s.cells = make([]Cell, n)
	s.cellsPrev = make([]Cell, n)
	s.cols, s.rows = c, r
}

// Clear 将后缓冲区中的每个单元格重置为空白单宽度单元格。
func (s *Screen) Clear() {
	for i := range s.cells {
		s.cells[i] = Cell{W: 1}
	}
}

// FillBg 用半块字符与逐格背景色快速填充整个屏幕，跳过 SetCell 的边界检查与宽字符处理。当背景图覆盖全屏时代替 Clear+逐格写入，减少一次全量遍历。
func (s *Screen) FillBg(w, h int, topColors, botColors [][]style.Color) {
	for y := 0; y < h && y < s.rows; y++ {
		row := topColors[y]
		rowBot := botColors[y]
		base := y * s.cols
		for x := 0; x < w && x < s.cols; x++ {
			s.cells[base+x] = Cell{Ch: '▀', Fg: row[x], Bg: rowBot[x], W: 1}
		}
	}
}

// SetCell 在 (x,y) 处使用给定颜色写入一个 rune。宽字符会占据下一个单元格，该单元格标记为 W=-1。越界写入将被忽略。
func (s *Screen) SetCell(x, y int, ch rune, fg, bg style.Color) {
	if x < 0 || x >= s.cols || y < 0 || y >= s.rows {
		return
	}
	w := 1
	if ch != 0 {
		w = runewidth.RuneWidth(ch)
	}
	s.cells[y*s.cols+x] = Cell{Ch: ch, Fg: fg, Bg: bg, W: w}
	if w > 1 && x+1 < s.cols {
		s.cells[y*s.cols+x+1] = Cell{Ch: ch, W: -1}
	}
}

// SetCellWide 强制将 ch 当作双格宽字符写入：占据 (x,y) 与右侧一格，
// 右半格标记为延续单元（Flush 时跳过，由终端绘制该字形的右半部分）。
// 用于宽度有歧义（East Asian Ambiguous）的字符——某些终端按 2 格渲染，
// 此时必须按 2 格记账，否则字形右半会溢出并覆盖相邻单元的颜色。
func (s *Screen) SetCellWide(x, y int, ch rune, fg, bg style.Color) {
	if x < 0 || x >= s.cols || y < 0 || y >= s.rows {
		return
	}
	s.cells[y*s.cols+x] = Cell{Ch: ch, Fg: fg, Bg: bg, W: 2}
	if x+1 < s.cols {
		s.cells[y*s.cols+x+1] = Cell{Ch: ch, W: -1}
	}
}

func (s *Screen) writeInt(n int) {
	i := len(s.intBuf)
	for {
		i--
		s.intBuf[i] = byte('0' + byte(n%10))
		n /= 10
		if n == 0 {
			break
		}
	}
	s.buf.Write(s.intBuf[i:])
}

// Flush 发出最少的 VT 转义序列，使终端与后缓冲区保持一致，然后交换前后缓冲区。cur 控制光标显示。
func (s *Screen) Flush(cur style.Cursor) {
	s.buf.Reset()

	needHide := !cur.Visible

	if cur.Visible != s.prevCursorVisible {
		if !cur.Visible {
			s.buf.WriteString("\x1b[?25l")
		}
		s.prevCursorVisible = cur.Visible
	}

	s.buf.WriteString("\x1b[?2026h") // begin synchronized update

	for y := 0; y < s.rows; y++ {
		for x := 0; x < s.cols; x++ {
			idx := y*s.cols + x
			c := s.cells[idx]
			p := s.cellsPrev[idx]

			if c.W < 1 {
				continue
			}
			wasRightHalf := p.W < 1
			if !wasRightHalf && c == p {
				continue
			}

			s.buf.WriteString("\x1b[")
			s.writeInt(y + 1)
			s.buf.WriteByte(';')
			s.writeInt(x + 1)
			s.buf.WriteByte('H')

			if c.Fg.IsSet {
				s.buf.WriteString("\x1b[38;2;")
				s.writeInt(int(c.Fg.R))
				s.buf.WriteByte(';')
				s.writeInt(int(c.Fg.G))
				s.buf.WriteByte(';')
				s.writeInt(int(c.Fg.B))
				s.buf.WriteByte('m')
			} else {
				s.buf.WriteString("\x1b[39m")
			}

			if c.Bg.IsSet {
				s.buf.WriteString("\x1b[48;2;")
				s.writeInt(int(c.Bg.R))
				s.buf.WriteByte(';')
				s.writeInt(int(c.Bg.G))
				s.buf.WriteByte(';')
				s.writeInt(int(c.Bg.B))
				s.buf.WriteByte('m')
			} else {
				s.buf.WriteString("\x1b[49m")
			}

			if c.Ch == 0 {
				s.buf.WriteByte(' ')
			} else {
				s.buf.WriteRune(c.Ch)
			}
		}
	}

	s.buf.WriteString("\x1b[0m")

	if cur.Visible {
		s.buf.WriteString("\x1b[?25h")
		s.buf.WriteString("\x1b[")
		s.writeInt(cur.Row + 1)
		s.buf.WriteByte(';')
		s.writeInt(cur.Col + 1)
		s.buf.WriteByte('H')
		s.prevCursorVisible = true
	} else if needHide {
		s.buf.WriteString("\x1b[?25l")
		s.prevCursorVisible = false
	}

	s.buf.WriteString("\x1b[?2026l") // end synchronized update

	io.WriteString(s.out, s.buf.String())

	s.cells, s.cellsPrev = s.cellsPrev, s.cells
}

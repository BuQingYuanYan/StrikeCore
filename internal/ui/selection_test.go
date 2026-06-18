package ui

import "testing"

func TestColAtCellX(t *testing.T) {
	tests := []struct {
		name string
		text string
		rel  int
		want int
	}{
		{"ascii start", "hello", 0, 0},
		{"ascii mid", "hello", 2, 2},
		{"ascii end", "hello", 5, 5},
		{"ascii past end", "hello", 99, 5},
		{"negative", "hello", -3, 0},
		// 宽字符："你好" 显示宽 4。列0->rune0，列1(你右半)->rune0，列2->rune1。
		{"cjk col0", "你好", 0, 0},
		{"cjk col1 right half", "你好", 1, 0},
		{"cjk col2", "你好", 2, 1},
		{"cjk col3 right half", "你好", 3, 1},
		{"cjk end", "你好", 4, 2},
		// 混合 "a你b"：a(1) 你(2) b(1)。列1->你(rune1)，列2(你右半)->rune1，列3->b(rune2)。
		{"mixed col1", "a你b", 1, 1},
		{"mixed col2 right half", "a你b", 2, 1},
		{"mixed col3", "a你b", 3, 2},
		{"empty", "", 0, 0},
		{"empty past", "", 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colAtCellX(tt.text, tt.rel); got != tt.want {
				t.Errorf("colAtCellX(%q,%d)=%d, want %d", tt.text, tt.rel, got, tt.want)
			}
		})
	}
}

func TestCellSpanForCols(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		c0, c1         int
		wantXS, wantXE int
	}{
		{"ascii whole", "hello", 0, 5, 0, 5},
		{"ascii sub", "hello", 1, 3, 1, 3},
		{"ascii empty", "hello", 2, 2, 2, 2},
		// "你好"：rune0..1 -> 列0..2。
		{"cjk first", "你好", 0, 1, 0, 2},
		{"cjk both", "你好", 0, 2, 0, 4},
		{"cjk second", "你好", 1, 2, 2, 4},
		// "a你b"：rune1..2(你) -> 列1..3。
		{"mixed cjk", "a你b", 1, 2, 1, 3},
		{"clamp hi", "abc", 0, 99, 0, 3},
		{"clamp lo", "abc", -2, 2, 0, 2},
		{"reversed", "abc", 3, 1, 1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xs, xe := cellSpanForCols(tt.text, tt.c0, tt.c1)
			if xs != tt.wantXS || xe != tt.wantXE {
				t.Errorf("cellSpanForCols(%q,%d,%d)=(%d,%d), want (%d,%d)",
					tt.text, tt.c0, tt.c1, xs, xe, tt.wantXS, tt.wantXE)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	a := SelPos{LineID: 5, Col: 2}
	b := SelPos{LineID: 8, Col: 0}
	// 正向。
	s := Selection{Anchor: a, Caret: b, Active: true}
	if start, end := s.Normalize(); start != a || end != b {
		t.Errorf("forward normalize = (%v,%v), want (%v,%v)", start, end, a, b)
	}
	// 反向拖拽：caret 在 anchor 之前。
	s = Selection{Anchor: b, Caret: a, Active: true}
	if start, end := s.Normalize(); start != a || end != b {
		t.Errorf("reverse normalize = (%v,%v), want (%v,%v)", start, end, a, b)
	}
	// 同行反向。
	p := SelPos{LineID: 3, Col: 1}
	q := SelPos{LineID: 3, Col: 7}
	s = Selection{Anchor: q, Caret: p, Active: true}
	if start, end := s.Normalize(); start != p || end != q {
		t.Errorf("same-line reverse = (%v,%v), want (%v,%v)", start, end, p, q)
	}
	// 跨区域：会话流 lineID 小于输入框 lineID（inputLineBase+li）。
	stream := SelPos{LineID: 2, Col: 0}
	input := SelPos{LineID: inputLineBase + 0, Col: 3}
	s = Selection{Anchor: input, Caret: stream, Active: true}
	if start, end := s.Normalize(); start != stream || end != input {
		t.Errorf("cross-region normalize = (%v,%v), want stream<input", start, end)
	}
}

func TestSelectedTextFromLines(t *testing.T) {
	// 会话流三行 + 输入框两行，lineID 体现区域顺序。
	lines := []selectableLine{
		{lineID: 1, sy: 1, x0: 5, text: "hello world"},
		{lineID: 2, sy: 2, x0: 5, text: "second line"},
		{lineID: 4, sy: 3, x0: 5, text: "third 你好"},
		{lineID: inputLineBase + 0, sy: 5, x0: 10, text: "input one"},
		{lineID: inputLineBase + 1, sy: 6, x0: 10, text: "input two"},
	}

	t.Run("single line", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{1, 0}, Caret: SelPos{1, 5}, Active: true}
		if got := SelectedTextFromLines(lines, sel); got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("cross line", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{1, 6}, Caret: SelPos{2, 6}, Active: true}
		want := "world\nsecond"
		if got := SelectedTextFromLines(lines, sel); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("reverse drag", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{2, 6}, Caret: SelPos{1, 6}, Active: true}
		want := "world\nsecond"
		if got := SelectedTextFromLines(lines, sel); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("cjk end col", func(t *testing.T) {
		// "third 你好" runes: t h i r d ' ' 你 好 -> len 8，全选。
		sel := Selection{Anchor: SelPos{4, 0}, Caret: SelPos{4, 8}, Active: true}
		if got := SelectedTextFromLines(lines, sel); got != "third 你好" {
			t.Errorf("got %q, want %q", got, "third 你好")
		}
	})

	t.Run("cross region stream to input", func(t *testing.T) {
		// 从会话流末行跨到输入框首行。
		sel := Selection{Anchor: SelPos{4, 6}, Caret: SelPos{inputLineBase + 0, 5}, Active: true}
		want := "你好\ninput"
		if got := SelectedTextFromLines(lines, sel); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("cross region reverse", func(t *testing.T) {
		// 反向拖：从输入框第二行回到会话流第二行。
		sel := Selection{Anchor: SelPos{inputLineBase + 1, 5}, Caret: SelPos{2, 7}, Active: true}
		want := "line\nthird 你好\ninput one\ninput"
		if got := SelectedTextFromLines(lines, sel); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("input only", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{inputLineBase + 0, 6}, Caret: SelPos{inputLineBase + 1, 5}, Active: true}
		want := "one\ninput"
		if got := SelectedTextFromLines(lines, sel); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("inactive", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{1, 0}, Caret: SelPos{1, 5}}
		if got := SelectedTextFromLines(lines, sel); got != "" {
			t.Errorf("inactive should be empty, got %q", got)
		}
	})

	t.Run("col overflow clamped", func(t *testing.T) {
		sel := Selection{Anchor: SelPos{1, 0}, Caret: SelPos{1, 999}, Active: true}
		if got := SelectedTextFromLines(lines, sel); got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})
}

func TestHitLine(t *testing.T) {
	// 会话流行 x0=5（1-based），输入框行 x0=10，验证两区域不同起列。
	lines := []selectableLine{
		{lineID: 1, sy: 2, x0: 5, text: "hello world"},
		{lineID: 2, sy: 3, x0: 5, text: "你好abc"},
		{lineID: inputLineBase + 0, sy: 5, x0: 10, text: "abc"},
	}

	// sy=2，sx=5 -> rel0 -> col0。
	if p, ok := hitLine(lines, 5, 2); !ok || p.LineID != 1 || p.Col != 0 {
		t.Errorf("hit (5,2)=%+v ok=%v, want lineID1 col0", p, ok)
	}
	// sx=10 -> rel5 -> col5（"hello"后）。
	if p, ok := hitLine(lines, 10, 2); !ok || p.LineID != 1 || p.Col != 5 {
		t.Errorf("hit (10,2)=%+v ok=%v, want lineID1 col5", p, ok)
	}
	// 未登记的屏幕行（sy=1）不命中。
	if _, ok := hitLine(lines, 5, 1); ok {
		t.Error("unregistered row should not hit")
	}
	// 越界行不命中。
	if _, ok := hitLine(lines, 5, 99); ok {
		t.Error("out-of-range row should not hit")
	}
	// CJK 行 sy=3：rel0->你(col0)，rel4->a(col2)。
	if p, ok := hitLine(lines, 5, 3); !ok || p.Col != 0 {
		t.Errorf("cjk hit rel0=%+v, want col0", p)
	}
	if p, ok := hitLine(lines, 9, 3); !ok || p.Col != 2 {
		t.Errorf("cjk hit rel4=%+v, want col2", p)
	}
	// 输入框行 x0=10：sx=10 -> rel0 -> col0，确认两区域 x0 不同。
	if p, ok := hitLine(lines, 10, 5); !ok || p.LineID != inputLineBase || p.Col != 0 {
		t.Errorf("input hit (10,5)=%+v ok=%v, want inputLineBase col0", p, ok)
	}
	// 输入框行用会话流 x0(5) 命中会错位：sx=5 在 x0=10 之前 -> rel<0 -> col0。
	if p, ok := hitLine(lines, 5, 5); !ok || p.Col != 0 {
		t.Errorf("input hit before x0=%+v, want col0", p)
	}
}

func TestSpanForLine(t *testing.T) {
	sel := Selection{Anchor: SelPos{1, 2}, Caret: SelPos{3, 4}, Active: true}

	// 首行：从 c0=2 到行末。
	c0, c1, ok := spanForLine(sel, 1, len([]rune("hello")))
	if !ok || c0 != 2 || c1 != 5 {
		t.Errorf("first line span=(%d,%d,%v), want (2,5,true)", c0, c1, ok)
	}
	// 中间行：整行。
	c0, c1, ok = spanForLine(sel, 2, len([]rune("abc")))
	if !ok || c0 != 0 || c1 != 3 {
		t.Errorf("middle span=(%d,%d,%v), want (0,3,true)", c0, c1, ok)
	}
	// 末行：从 0 到 c1=4。
	c0, c1, ok = spanForLine(sel, 3, len([]rune("world!!")))
	if !ok || c0 != 0 || c1 != 4 {
		t.Errorf("last span=(%d,%d,%v), want (0,4,true)", c0, c1, ok)
	}
	// 区间外。
	if _, _, ok := spanForLine(sel, 0, 1); ok {
		t.Error("line 0 outside selection should be ok=false")
	}
	if _, _, ok := spanForLine(sel, 5, 1); ok {
		t.Error("line 5 outside selection should be ok=false")
	}
	// 非激活选区。
	sel.Active = false
	if _, _, ok := spanForLine(sel, 2, 3); ok {
		t.Error("inactive selection should be ok=false")
	}
}

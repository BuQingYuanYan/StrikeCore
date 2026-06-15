package editor

import (
	"testing"

	"strike-core/internal/input"
)

func typeStr(e *Editor, s string) {
	for _, r := range s {
		e.HandleKey(input.KeyRune, r)
	}
}

func TestInsertAndString(t *testing.T) {
	e := &Editor{}
	typeStr(e, "hello")
	if got := e.String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
	if e.Cursor() != 5 {
		t.Errorf("cursor = %d, want 5", e.Cursor())
	}
}

func TestInsertMidline(t *testing.T) {
	e := &Editor{}
	typeStr(e, "helo")
	e.HandleKey(input.KeyLeft, 0) // before o
	e.HandleKey(input.KeyLeft, 0) // before l
	e.HandleKey(input.KeyRune, 'l')
	if got := e.String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
}

func TestBackspaceAndDelete(t *testing.T) {
	e := &Editor{}
	typeStr(e, "abc")
	e.HandleKey(input.KeyBackspace, 0)
	if got := e.String(); got != "ab" {
		t.Errorf("after backspace = %q, want %q", got, "ab")
	}
	e.HandleKey(input.KeyHome, 0)
	e.HandleKey(input.KeyDelete, 0)
	if got := e.String(); got != "b" {
		t.Errorf("after delete = %q, want %q", got, "b")
	}
}

func TestHomeEnd(t *testing.T) {
	e := &Editor{}
	typeStr(e, "abcdef")
	e.HandleKey(input.KeyHome, 0)
	if e.Cursor() != 0 {
		t.Errorf("home cursor = %d, want 0", e.Cursor())
	}
	e.HandleKey(input.KeyEnd, 0)
	if e.Cursor() != 6 {
		t.Errorf("end cursor = %d, want 6", e.Cursor())
	}
}

func TestQuit(t *testing.T) {
	e := &Editor{}
	if !e.HandleKey(input.KeyQuit, 0) {
		t.Error("KeyQuit should return quit=true")
	}
}

func TestWrapLines(t *testing.T) {
	e := &Editor{}
	typeStr(e, "abcdefghij") // 10 single-width chars
	starts := e.WrapLines(4)
	want := []int{0, 4, 8}
	if len(starts) != len(want) {
		t.Fatalf("starts = %v, want %v", starts, want)
	}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("starts = %v, want %v", starts, want)
		}
	}
}

func TestWrapWideChars(t *testing.T) {
	e := &Editor{}
	typeStr(e, "世界你好") // 4 wide (W=2) chars => width 8, 2 per line
	starts := e.WrapLines(4)
	want := []int{0, 2}
	if len(starts) != len(want) {
		t.Fatalf("starts = %v, want %v", starts, want)
	}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("starts = %v, want %v", starts, want)
		}
	}
}

func TestMoveVert(t *testing.T) {
	e := &Editor{}
	e.SetInputW(4)
	typeStr(e, "abcdefgh") // wraps to lines [abcd][efgh], starts [0,4]
	// Place cursor at index 5 (line 1, col 1) — away from the wrap boundary.
	e.HandleKey(input.KeyHome, 0)
	for i := 0; i < 5; i++ {
		e.HandleKey(input.KeyRight, 0)
	}
	e.HandleKey(input.KeyUp, 0)
	starts := e.WrapLines(4)
	line, col := e.CursorPos(starts, 4)
	if line != 0 || col != 1 {
		t.Errorf("after up, line=%d col=%d, want 0,1", line, col)
	}
	e.HandleKey(input.KeyDown, 0)
	starts = e.WrapLines(4)
	line, col = e.CursorPos(starts, 4)
	if line != 1 || col != 1 {
		t.Errorf("after down, line=%d col=%d, want 1,1", line, col)
	}
}

func TestEnsureVisible(t *testing.T) {
	e := &Editor{}
	// 5 total lines, viewport of 3. Cursor on line 4 should scroll to show it.
	e.EnsureVisible(4, 5, 3)
	if e.ScrollLine() != 2 {
		t.Errorf("scrollLine = %d, want 2", e.ScrollLine())
	}
	// Cursor back to line 0 scrolls up.
	e.EnsureVisible(0, 5, 3)
	if e.ScrollLine() != 0 {
		t.Errorf("scrollLine = %d, want 0", e.ScrollLine())
	}
}

func TestCursorClampsOnEmpty(t *testing.T) {
	e := &Editor{}
	e.HandleKey(input.KeyLeft, 0)
	e.HandleKey(input.KeyBackspace, 0)
	if e.Cursor() != 0 || e.Len() != 0 {
		t.Errorf("empty editor cursor=%d len=%d, want 0,0", e.Cursor(), e.Len())
	}
}

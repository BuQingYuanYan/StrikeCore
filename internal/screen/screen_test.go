package screen

import (
	"bytes"
	"strings"
	"testing"

	"strike-core/internal/style"
)

func TestFlushEmitsSyncMarkers(t *testing.T) {
	var out bytes.Buffer
	s := New(&out)
	s.Realloc(4, 2)
	s.Clear()
	s.Flush(style.Cursor{Visible: false})

	got := out.String()
	if !strings.Contains(got, "\x1b[?2026h") {
		t.Errorf("output missing synchronized-update start marker, got %q", got)
	}
	if !strings.Contains(got, "\x1b[?2026l") {
		t.Error("output missing synchronized-update end marker")
	}
	// The sync-start must precede the sync-end.
	if strings.Index(got, "\x1b[?2026h") > strings.LastIndex(got, "\x1b[?2026l") {
		t.Error("synchronized-update markers out of order")
	}
}

func TestFlushDiffOnly(t *testing.T) {
	var out bytes.Buffer
	s := New(&out)
	s.Realloc(4, 1)
	s.Clear()
	s.SetCell(0, 0, 'A', style.RGB(255, 0, 0), style.Color{})
	s.Flush(style.Cursor{Visible: false})

	out.Reset()
	// Same content -> nothing but the sync wrapper and reset should be emitted.
	s.Clear()
	s.SetCell(0, 0, 'A', style.RGB(255, 0, 0), style.Color{})
	s.Flush(style.Cursor{Visible: false})

	got := out.String()
	if strings.Contains(got, "A") {
		t.Errorf("unchanged cell should not be re-emitted, got %q", got)
	}
}

func TestFlushEmitsChangedCell(t *testing.T) {
	var out bytes.Buffer
	s := New(&out)
	s.Realloc(4, 1)
	s.Clear()
	s.Flush(style.Cursor{Visible: false})

	out.Reset()
	s.Clear()
	s.SetCell(1, 0, 'Z', style.RGB(10, 20, 30), style.RGB(40, 50, 60))
	s.Flush(style.Cursor{Visible: false})

	got := out.String()
	if !strings.Contains(got, "Z") {
		t.Errorf("changed cell rune missing, got %q", got)
	}
	if !strings.Contains(got, "\x1b[1;2H") {
		t.Errorf("changed cell should carry a cursor-position escape to row1 col2, got %q", got)
	}
	if !strings.Contains(got, "38;2;10;20;30") {
		t.Errorf("fg truecolor escape missing, got %q", got)
	}
	if !strings.Contains(got, "48;2;40;50;60") {
		t.Errorf("bg truecolor escape missing, got %q", got)
	}
}

func TestFlushCursorSequence(t *testing.T) {
	var out bytes.Buffer
	s := New(&out)
	s.Realloc(8, 4)
	s.Clear()
	s.Flush(style.Cursor{Row: 2, Col: 3, Visible: true})

	got := out.String()
	if !strings.Contains(got, "\x1b[?25h") {
		t.Error("visible cursor should emit show-cursor escape")
	}
	if !strings.Contains(got, "\x1b[3;4H") {
		t.Errorf("cursor should be positioned at row3 col4 (1-based), got %q", got)
	}
}

func TestSetCellWideRune(t *testing.T) {
	s := New(&bytes.Buffer{})
	s.Realloc(4, 1)
	s.Clear()
	s.SetCell(0, 0, '世', style.Color{}, style.Color{})
	if s.cells[0].W != 2 {
		t.Errorf("wide rune width = %d, want 2", s.cells[0].W)
	}
	if s.cells[1].W != -1 {
		t.Errorf("right-half marker width = %d, want -1", s.cells[1].W)
	}
}

package input

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		wantCode int
		wantRune rune
		wantN    int
	}{
		{"empty", nil, KeyNone, 0, 0},
		{"up", []byte("\x1b[A"), KeyUp, 0, 3},
		{"down", []byte("\x1b[B"), KeyDown, 0, 3},
		{"right", []byte("\x1b[C"), KeyRight, 0, 3},
		{"left", []byte("\x1b[D"), KeyLeft, 0, 3},
		{"home csi H", []byte("\x1b[H"), KeyHome, 0, 3},
		{"end csi F", []byte("\x1b[F"), KeyEnd, 0, 3},
		{"home ss3 H", []byte("\x1bOH"), KeyHome, 0, 3},
		{"end ss3 F", []byte("\x1bOF"), KeyEnd, 0, 3},
		{"home 1~", []byte("\x1b[1~"), KeyHome, 0, 4},
		{"delete 3~", []byte("\x1b[3~"), KeyDelete, 0, 4},
		{"end 4~", []byte("\x1b[4~"), KeyEnd, 0, 4},
		{"lone esc", []byte("\x1b"), KeyEscape, 0, 1},
		{"esc then non-bracket", []byte("\x1bZ"), KeyEscape, 0, 1},
		{"partial csi", []byte("\x1b["), KeyEscape, 0, 1},
		{"backspace del", []byte("\x7f"), KeyBackspace, 0, 1},
		{"backspace bs", []byte("\x08"), KeyBackspace, 0, 1},
		{"ctrl-c quit", []byte("\x03"), KeyQuit, 0, 1},
		{"enter cr", []byte("\r"), KeyEnter, 0, 1},
		{"enter lf", []byte("\n"), KeyEnter, 0, 1},
		{"ascii rune", []byte("a"), KeyRune, 'a', 1},
		{"unknown control", []byte("\x01"), KeyNone, 0, 0},
		{"cjk rune", []byte("世"), KeyRune, '世', 3},
		{"incomplete utf8 tail", []byte{0xE4, 0xB8}, KeyNone, 0, 0},
		{"sgr wheel up", []byte("\x1b[<64;10;20M"), KeyScrollUp, 0, 12},
		{"sgr wheel down", []byte("\x1b[<65;10;20M"), KeyScrollDown, 0, 12},
		{"sgr left click", []byte("\x1b[<0;5;5M"), KeyNone, 0, 9},
		{"sgr release", []byte("\x1b[<0;5;5m"), KeyNone, 0, 9},
		{"sgr wheel down release", []byte("\x1b[<65;10;20m"), KeyNone, 0, 12},
		{"sgr incomplete", []byte("\x1b[<64;10;2"), KeyNone, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, r, n := Parse(tt.in)
			if code != tt.wantCode || r != tt.wantRune || n != tt.wantN {
				t.Errorf("Parse(%q) = (%d,%q,%d), want (%d,%q,%d)",
					tt.in, code, r, n, tt.wantCode, tt.wantRune, tt.wantN)
			}
		})
	}
}

// A buffer holding two keys back to back should be consumed one key at a time.
func TestParseSequential(t *testing.T) {
	buf := []byte("\x1b[Aab")
	var got []int
	for len(buf) > 0 {
		code, _, n := Parse(buf)
		if n == 0 {
			t.Fatalf("stuck with %d bytes remaining", len(buf))
		}
		got = append(got, code)
		buf = buf[n:]
	}
	want := []int{KeyUp, KeyRune, KeyRune}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

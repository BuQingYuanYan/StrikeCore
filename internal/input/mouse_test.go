package input

import "testing"

func TestParseSGRMouse(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		wantOK  bool
		wantN   int
		wantX   int
		wantY   int
		wantKnd MouseKind
	}{
		// 左键按下：Cb=0，'M'。坐标 10;20 -> 0-based 9,19。
		{"press", []byte("\x1b[<0;10;20M"), true, 11, 9, 19, MousePress},
		// 左键释放：'m'。
		{"release", []byte("\x1b[<0;10;20m"), true, 11, 9, 19, MouseRelease},
		// 左键拖拽：Cb=32(bit5)。
		{"drag", []byte("\x1b[<32;3;4M"), true, 10, 2, 3, MouseDrag},
		// 滚轮上/下：Cb=64/65。
		{"wheel up", []byte("\x1b[<64;1;1M"), true, 10, 0, 0, MouseWheelUp},
		{"wheel down", []byte("\x1b[<65;1;1M"), true, 10, 0, 0, MouseWheelDown},
		// 右键按下(Cb=2) -> Other。
		{"right press", []byte("\x1b[<2;5;5M"), true, 9, 4, 4, MouseOther},
		// 中键拖拽(Cb=33: bit5+button1) -> Other（非左键拖拽）。
		{"mid drag", []byte("\x1b[<33;5;5M"), true, 10, 4, 4, MouseOther},
		// 滚轮释放('m') 视为 Other（不重复触发滚动）。
		{"wheel release", []byte("\x1b[<64;1;1m"), true, 10, 0, 0, MouseOther},
		// 大坐标多位数。
		{"large coords", []byte("\x1b[<0;123;456M"), true, 13, 122, 455, MousePress},

		// 不完整：缺终结符。
		{"incomplete no final", []byte("\x1b[<0;10;20"), false, 0, 0, 0, MouseOther},
		// 不完整：只有部分参数。
		{"incomplete partial", []byte("\x1b[<0;1"), false, 0, 0, 0, MouseOther},
		// 不完整：仅前缀。
		{"incomplete prefix", []byte("\x1b[<"), false, 0, 0, 0, MouseOther},
		// 非鼠标序列前缀。
		{"not sgr mouse", []byte("\x1b[A"), false, 0, 0, 0, MouseOther},
		{"too short", []byte("\x1b["), false, 0, 0, 0, MouseOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok, n := ParseSGRMouse(tt.in)
			if ok != tt.wantOK || n != tt.wantN {
				t.Fatalf("ParseSGRMouse(%q) ok=%v n=%d, want ok=%v n=%d",
					tt.in, ok, n, tt.wantOK, tt.wantN)
			}
			if !ok {
				return
			}
			if ev.X != tt.wantX || ev.Y != tt.wantY || ev.Kind != tt.wantKnd {
				t.Errorf("ParseSGRMouse(%q) = {X:%d Y:%d Kind:%d}, want {X:%d Y:%d Kind:%d}",
					tt.in, ev.X, ev.Y, ev.Kind, tt.wantX, tt.wantY, tt.wantKnd)
			}
		})
	}
}

// Package style 持有在渲染栈中共享的展示原语：RGB 颜色、
// 光标模型和颜色主题。它是一个叶子包（不导入任何 internal 包），
// 以便 screen、ui 和 config 都可以依赖它而不会产生导入循环。
package style

import (
	"fmt"
	"strconv"
	"strings"
)

// Color 是一个真彩色 RGB 值。IsSet 区分显式颜色与零值，
// 零值渲染为终端的默认前景色/背景色。
type Color struct {
	R, G, B uint8
	IsSet   bool
}

// RGB 构造一个显式颜色。
func RGB(r, g, b uint8) Color {
	return Color{R: r, G: g, B: b, IsSet: true}
}

// Scale 返回每个通道乘以 f 后的颜色（限制在 [0,255] 范围内）。
// 用于背景变暗。
func (c Color) Scale(f float64) Color {
	if !c.IsSet {
		return c
	}
	return Color{
		R:     scaleChannel(c.R, f),
		G:     scaleChannel(c.G, f),
		B:     scaleChannel(c.B, f),
		IsSet: true,
	}
}

func scaleChannel(v uint8, f float64) uint8 {
	x := float64(v) * f
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}

// Blend 把 c 朝 other 混合，比例为 t（取值 [0,1]）：t=0 返回 c，t=1 返回 other。
// 任一颜色未设置时返回另一个（无意义的混合直接退化）。常用于把气泡的纯色背景
// 按透明度与下方背景图颜色混合。
func (c Color) Blend(other Color, t float64) Color {
	if t <= 0 || !other.IsSet {
		return c
	}
	if t >= 1 || !c.IsSet {
		return other
	}
	return Color{
		R:     uint8(float64(c.R)*(1-t) + float64(other.R)*t + 0.5),
		G:     uint8(float64(c.G)*(1-t) + float64(other.G)*t + 0.5),
		B:     uint8(float64(c.B)*(1-t) + float64(other.B)*t + 0.5),
		IsSet: true,
	}
}

// ParseHex 将 "#rrggbb" 或 "rrggbb" 解析为 Color。
func ParseHex(s string) (Color, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return Color{}, fmt.Errorf("style: invalid hex color %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("style: invalid hex color %q: %w", s, err)
	}
	return Color{
		R:     uint8(v >> 16),
		G:     uint8(v >> 8),
		B:     uint8(v),
		IsSet: true,
	}, nil
}

// Cursor 是文本光标应显示的位置（及是否显示）的渲染结果。
// 由视图渲染返回并由 Screen.Flush 消费，取代了旧的全局光标侧信道。
type Cursor struct {
	Row, Col int
	Visible  bool
}

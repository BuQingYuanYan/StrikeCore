package style

// Theme 持有 UI 使用的所有颜色。使用 DefaultTheme() 或从 config 派生。
type Theme struct {
	ArtLeft         Color
	ArtRight        Color
	InputAreaBg     Color
	InputTextFg     Color
	PromptFg        Color
	PromptBg        Color
	BlockEdgeFg     Color
	BlockEdgeBg     Color
	UserEdgeFg      Color
	AssistantEdgeFg Color
	SepFg           Color
	DimFg           Color
	PlaceholderFg   Color
	HintFg          Color
	ModelFg         Color
	LogoDepthBg     Color
	SelectionBg     Color // 鼠标自绘选区高亮
}

// DefaultTheme 返回硬编码调色板。
func DefaultTheme() Theme {
	inputBg := RGB(0x1A, 0x1A, 0x1A)
	return Theme{
		ArtLeft:         RGB(0x60, 0xCD, 0xFF),
		ArtRight:        RGB(0x00, 0x78, 0xD4),
		InputAreaBg:     inputBg,
		InputTextFg:     RGB(0xD4, 0xD4, 0xD4),
		PromptFg:        RGB(0x60, 0xCD, 0xFF),
		PromptBg:        inputBg,
		BlockEdgeFg:     RGB(0x00, 0x78, 0xD4),
		BlockEdgeBg:     inputBg,
		UserEdgeFg:      RGB(0x60, 0xCD, 0xFF),
		AssistantEdgeFg: RGB(0x4E, 0xC9, 0x7A),
		SepFg:           RGB(0x88, 0x88, 0x88),
		DimFg:           RGB(0xA8, 0xA8, 0xA8),
		PlaceholderFg:   RGB(0x98, 0x98, 0x98),
		HintFg:          RGB(0xC8, 0xA0, 0x60),
		ModelFg:         RGB(0xE0, 0xE0, 0xE0),
		LogoDepthBg:     RGB(0x2A, 0x3A, 0x5C),
		SelectionBg:     RGB(0x33, 0x55, 0x99),
	}
}

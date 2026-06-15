package ui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"strike-core/internal/style"
)

//go:embed assets/bg.png
var embeddedBg []byte

// Background 在 UI 后面渲染半块风格的图像，并提供逐格颜色，
// 以便文本可以透明地绘制在其上方。它取代了旧 background.go 中的包全局变量。
type Background struct {
	path       string
	brightness float64

	raw *image.NRGBA

	// preloaded 接收在后台 goroutine 中解码的下一张壁纸图像，
	// 以便 Activate 可以无卡顿地切换。
	preloaded chan *image.NRGBA

	// 由 drawBgImage / BotColor 使用的激活颜色网格。这些指向当前在插槽 0 中的
	// LRU 缓存条目。
	topColors [][]style.Color
	botColors [][]style.Color

	// 3 条目 LRU 缓存，键为 (w, h, brightness)。插槽 0 是最近使用的，
	// 插槽 2 是驱逐候选。
	cache [3]bgCacheEntry
}

// bgCacheEntry 保存一个预先计算好的终端尺寸颜色网格。
type bgCacheEntry struct {
	w, h       int
	brightness float64
	topColors  [][]style.Color
	botColors  [][]style.Color
}

// bgExts 列出 ResolveBgDir 可识别的图片文件扩展名。
var bgExts = []string{".png", ".jpg", ".jpeg"}

// BgDirCfg 是 backgrounds 目录下 (config.json) 中的可选 JSON 配置。
// 允许用户无需编辑主配置文件即可控制轮播行为和当前壁纸。
type BgDirCfg struct {
	Enabled          *bool    `json:"enabled,omitempty"`
	Interval         *int     `json:"interval,omitempty"`  // 秒
	Wallpaper        *string  `json:"wallpaper,omitempty"` // 文件名
	BubbleBgOpacity  *float64 `json:"bubble_bg_opacity,omitempty"` // 气泡背景透明度 0..1
	Brightness       *float64 `json:"brightness,omitempty"` // 图片亮度 0..1
}

// defaultDirCfgJSON 在首次运行时写入 backgrounds/config.json，
// 以便用户无需编辑主配置文件即可调整行为。额外字段 (_说明)
// 会被 JSON 解码器静默忽略，并作为内联文档。
const defaultDirCfgJSON = `{
    "_说明": "enabled=true开启轮播/false关闭轮播   interval=轮播间隔(秒)   wallpaper=指定图片文件名(留空则用文件夹第一张)   bubble_bg_opacity=气泡背景透明度0~1(0=纯色不透明, 数值越大越透明越能看到背景图, 1=完全透明)   brightness=图片亮度0~1(0=最暗, 1=最亮)",
    "enabled": true,
    "interval": 60,
    "wallpaper": "",
    "bubble_bg_opacity": 0,
    "brightness": 0.35
}
`

// ReadBgDirCfg 加载 backgrounds/config.json。如果文件不存在，
// 则使用合理的默认值创建，以便用户无需手动设置即可发现选项。
// 解析错误返回零值（调用方自行应用默认值）。
func ReadBgDirCfg(dir string) BgDirCfg {
	os.MkdirAll(dir, 0755)
	cfgPath := filepath.Join(dir, "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		// 文件缺失 — 写入默认值。
		if err2 := os.WriteFile(cfgPath, []byte(defaultDirCfgJSON), 0644); err2 == nil {
			raw = []byte(defaultDirCfgJSON)
		} else {
			return BgDirCfg{}
		}
	}
	var c BgDirCfg
	if err := json.Unmarshal(raw, &c); err != nil {
		return BgDirCfg{}
	}
	return c
}

// ListBgImages 扫描目录中可识别的图片文件，并按字典顺序返回完整路径。
// 如果目录不存在则创建。
func ListBgImages(dir string) []string {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, candidate := range bgExts {
			if ext == candidate {
				out = append(out, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	return out
}

// ResolveBgDir 确保目录存在，如果包含任何可识别的图片文件，
// 则返回第一个文件的路径（字典顺序）。目录为空或不包含任何支持的图片时返回 ""。
func ResolveBgDir(dir string) string {
	paths := ListBgImages(dir)
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// Preload 在后台 goroutine 中解码图片文件，并将结果发送到 preloaded 通道，
// 以便 Activate 可以立即切换。
func (b *Background) Preload(path string) {
	go func() {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return
		}
		select {
		case b.preloaded <- toNRGBA(img):
		default:
		}
	}()
}

// Activate 将预解码的图像切换到 raw 并清除缓存。
// 当没有预加载的图像可用时返回 false（调用方应回退到 Load）。
func (b *Background) Activate(path string) bool {
	select {
	case img := <-b.preloaded:
		b.raw = img
		b.path = path
		b.topColors = nil
		b.botColors = nil
		clear(b.cache[:])
		return true
	default:
		return false
	}
}

// Load 同步地将 Background 切换到不同的图片文件。
// LRU 缓存被清除，以便下次 ensure() 调用重新计算颜色网格。
// 成功返回 true，文件无法解码返回 false。
func (b *Background) Load(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return false
	}
	b.raw = toNRGBA(img)
	b.path = path
	b.topColors = nil
	b.botColors = nil
	clear(b.cache[:])
	return true
}

// Path 返回当前加载的图片文件路径。
func (b *Background) Path() string { return b.path }

// SetBrightness 更新亮度并清除缓存，使下一次渲染使用新值。
func (b *Background) SetBrightness(v float64) {
	b.brightness = v
	b.topColors = nil
	b.botColors = nil
	clear(b.cache[:])
}

// NewBackground 构建一个 Background。如果路径非空且可加载则使用；
// 否则解码嵌入式资源。nil 的 raw 图像（两者都失败）会优雅降级：
// Draw 是空操作，BotColor 返回零颜色。
func NewBackground(path string, brightness float64) *Background {
	b := &Background{
		path:       path,
		brightness: brightness,
		preloaded:  make(chan *image.NRGBA, 1),
	}
	b.raw = b.decode()
	return b
}

func (b *Background) decode() *image.NRGBA {
	if b.path != "" {
		if f, err := os.Open(b.path); err == nil {
			defer f.Close()
			if img, _, err := image.Decode(f); err == nil {
				return toNRGBA(img)
			}
		}
		// 任何外部失败时回退到嵌入式资源。
	}
	if len(embeddedBg) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(embeddedBg))
	if err != nil {
		return nil
	}
	return toNRGBA(img)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	bounds := img.Bounds()
	n := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			n.Set(x, y, img.At(x, y))
		}
	}
	return n
}

// ensure 为 w×h 终端重新计算逐格颜色网格。
// 结果缓存在 3 条目 LRU 中，因此在两种尺寸之间切换（例如最大化/还原）可避免冗余工作。
func (b *Background) ensure(w, h int) {
	if b.raw == nil {
		return
	}
	bright := b.brightness

	// 1. 从最近使用(0)到最久未使用(2)扫描 LRU 缓存。
	for i := 0; i < len(b.cache); i++ {
		e := &b.cache[i]
		if e.botColors != nil && e.w == w && e.h == h && e.brightness == bright {
			// 提升到插槽 0：交换条目使找到的移到前面。
			b.cache[0], b.cache[i] = b.cache[i], b.cache[0]
			b.topColors = b.cache[0].topColors
			b.botColors = b.cache[0].botColors
			return
		}
	}

	// 2. 缓存未命中：驱逐插槽 2，移动 1→2、0→1，计算结果写入插槽 0。
	b.cache[2] = b.cache[1]
	b.cache[1] = b.cache[0]

	bounds := b.raw.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	pix := b.raw.Pix
	stride := b.raw.Stride
	needRows := h * 2

	scale := max(float64(w)/float64(imgW), float64(needRows)/float64(imgH))
	offX := (float64(w) - float64(imgW)*scale) / 2
	offY := (float64(needRows) - float64(imgH)*scale) / 2

	tops := make([][]style.Color, h)
	bots := make([][]style.Color, h)
	for y := 0; y < h; y++ {
		tops[y] = make([]style.Color, w)
		bots[y] = make([]style.Color, w)
		for x := 0; x < w; x++ {
			fx := (float64(x) - offX) / scale
			fyTop := (float64(y*2) - offY) / scale
			fyBot := (float64(y*2+1) - offY) / scale
			tops[y][x] = style.RGB(sampleBilinear(pix, stride, imgW, imgH, fx, fyTop, bright))
			bots[y][x] = style.RGB(sampleBilinear(pix, stride, imgW, imgH, fx, fyBot, bright))
		}
	}

	b.cache[0] = bgCacheEntry{w: w, h: h, brightness: bright, topColors: tops, botColors: bots}
	b.topColors = tops
	b.botColors = bots
}

// sampleBilinear 使用双线性插值返回分数位置 (fx, fy) 处的 (r, g, b)，
// 亮度已预先应用到每个通道。
func sampleBilinear(pix []uint8, stride, imgW, imgH int, fx, fy, bright float64) (uint8, uint8, uint8) {
	if fx < 0 {
		fx = 0
	} else if fx >= float64(imgW-1) {
		fx = float64(imgW - 1)
	}
	if fy < 0 {
		fy = 0
	} else if fy >= float64(imgH-1) {
		fy = float64(imgH - 1)
	}

	ix := int(fx)
	iy := int(fy)
	fracX := fx - float64(ix)
	fracY := fy - float64(iy)
	x1 := min(ix, imgW-1)
	y1 := min(iy, imgH-1)
	dX := min(1, imgW-1-x1) * 4
	dY := min(1, imgH-1-y1) * stride

	base := y1*stride + x1*4
	w1 := (1 - fracX) * (1 - fracY)
	w2 := fracX * (1 - fracY)
	w3 := (1 - fracX) * fracY
	w4 := fracX * fracY

	r := (float64(pix[base])*w1 + float64(pix[base+dX])*w2 +
		float64(pix[base+dY])*w3 + float64(pix[base+dY+dX])*w4) * bright
	g := (float64(pix[base+1])*w1 + float64(pix[base+dX+1])*w2 +
		float64(pix[base+dY+1])*w3 + float64(pix[base+dY+dX+1])*w4) * bright
	b := (float64(pix[base+2])*w1 + float64(pix[base+dX+2])*w2 +
		float64(pix[base+dY+2])*w3 + float64(pix[base+dY+dX+2])*w4) * bright

	return uint8(r + 0.5), uint8(g + 0.5), uint8(b + 0.5)
}

// BotColor 返回单元格 (x,y) 的下半部分颜色，如果超出范围或未加载图像则返回零颜色。
func (b *Background) BotColor(x, y int) style.Color {
	if b.botColors != nil && y >= 0 && y < len(b.botColors) && x >= 0 && x < len(b.botColors[y]) {
		return b.botColors[y][x]
	}
	return style.Color{}
}

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"strike-core/internal/config"
	"strike-core/internal/screen"
	"strike-core/internal/ui"
)

// writeTestPNG 在 dir 下写入一张可被 image.Decode 解码的最小 PNG。
func writeTestPNG(t *testing.T, path string, c color.Color) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, c)
	img.Set(1, 0, c)
	img.Set(0, 1, c)
	img.Set(1, 1, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write png %s: %v", path, err)
	}
}

// newTestView 构造一个不依赖真实终端的 View（screen 写到 bytes.Buffer）。
func newTestView() (*ui.View, *ui.Background) {
	cfg := config.Default()
	s := screen.New(&bytes.Buffer{})
	bg := ui.NewBackground("", cfg.Brightness)
	view := ui.NewView(s, cfg, bg, "/work")
	return view, bg
}

// TestReloadCommand 验证 /reload 会重新读取 backgrounds/config.json，
// 刷新壁纸列表，并把亮度、轮播开关等设置即时应用到 cfg/bg。
func TestReloadCommand(t *testing.T) {
	t.Run("reload applies config and lists wallpapers", func(t *testing.T) {
		bgDir := t.TempDir()
		writeTestPNG(t, filepath.Join(bgDir, "a.png"), color.NRGBA{255, 0, 0, 255})
		writeTestPNG(t, filepath.Join(bgDir, "b.png"), color.NRGBA{0, 255, 0, 255})
		cfgJSON := `{"enabled": false, "interval": 30, "wallpaper": "b.png", "bubble_bg_opacity": 0.5, "brightness": 0.8}`
		if err := os.WriteFile(filepath.Join(bgDir, "config.json"), []byte(cfgJSON), 0644); err != nil {
			t.Fatal(err)
		}

		view, bg := newTestView()
		cfg := config.Default()
		var (
			messages    []ui.Message
			msgScroll   int
			bgImages    []string
			bgIndex     int
			bgSlideTick *time.Ticker
			bgSlideCh   <-chan time.Time
			slideReady  bool = true
		)

		handled := handleCommand("/reload", &messages, &msgScroll, view, bg, &bgImages,
			bgDir, &bgIndex, &bgSlideTick, &bgSlideCh, &slideReady, &cfg)

		if !handled {
			t.Fatal("/reload should be handled")
		}
		if len(bgImages) != 2 {
			t.Errorf("bgImages=%d, want 2", len(bgImages))
		}
		if cfg.Brightness != 0.8 {
			t.Errorf("brightness=%v, want 0.8", cfg.Brightness)
		}
		// wallpaper 指定 b.png，应当被加载为当前背景。
		if filepath.Base(bg.Path()) != "b.png" {
			t.Errorf("bg path=%q, want .../b.png", bg.Path())
		}
		// enabled=false 关闭轮播。
		if slideReady {
			t.Error("slideReady should be false when enabled=false")
		}
		if bgSlideTick != nil {
			t.Error("slide ticker should be nil when carousel disabled")
		}
	})

	t.Run("reload enables carousel ticker when enabled with multiple images", func(t *testing.T) {
		bgDir := t.TempDir()
		writeTestPNG(t, filepath.Join(bgDir, "a.png"), color.NRGBA{255, 0, 0, 255})
		writeTestPNG(t, filepath.Join(bgDir, "b.png"), color.NRGBA{0, 255, 0, 255})
		cfgJSON := `{"enabled": true, "interval": 5, "wallpaper": "", "bubble_bg_opacity": 0.2, "brightness": 0.5}`
		if err := os.WriteFile(filepath.Join(bgDir, "config.json"), []byte(cfgJSON), 0644); err != nil {
			t.Fatal(err)
		}

		view, bg := newTestView()
		cfg := config.Default()
		var (
			messages    []ui.Message
			msgScroll   int
			bgImages    []string
			bgIndex     int
			bgSlideTick *time.Ticker
			bgSlideCh   <-chan time.Time
			slideReady  bool
		)

		handleCommand("/reload", &messages, &msgScroll, view, bg, &bgImages,
			bgDir, &bgIndex, &bgSlideTick, &bgSlideCh, &slideReady, &cfg)

		if !slideReady {
			t.Error("slideReady should be true when enabled=true")
		}
		if bgSlideTick == nil {
			t.Error("slide ticker should be created for multiple images with carousel on")
		} else {
			bgSlideTick.Stop()
		}
		if cfg.BgInterval != 5*time.Second {
			t.Errorf("BgInterval=%v, want 5s", cfg.BgInterval)
		}
	})

	t.Run("unknown command appends help message", func(t *testing.T) {
		bgDir := t.TempDir()
		view, bg := newTestView()
		cfg := config.Default()
		var (
			messages    []ui.Message
			msgScroll   int
			bgImages    []string
			bgIndex     int
			bgSlideTick *time.Ticker
			bgSlideCh   <-chan time.Time
			slideReady  bool
		)

		handled := handleCommand("/bogus", &messages, &msgScroll, view, bg, &bgImages,
			bgDir, &bgIndex, &bgSlideTick, &bgSlideCh, &slideReady, &cfg)

		if !handled {
			t.Fatal("/bogus should be handled (consumed as command)")
		}
		if len(messages) != 1 || messages[0].Role != "assistant" {
			t.Fatalf("want 1 assistant help message, got %+v", messages)
		}
	})

	t.Run("non-slash input is not a command", func(t *testing.T) {
		bgDir := t.TempDir()
		view, bg := newTestView()
		cfg := config.Default()
		var (
			messages    []ui.Message
			msgScroll   int
			bgImages    []string
			bgIndex     int
			bgSlideTick *time.Ticker
			bgSlideCh   <-chan time.Time
			slideReady  bool
		)

		if handleCommand("hello world", &messages, &msgScroll, view, bg, &bgImages,
			bgDir, &bgIndex, &bgSlideTick, &bgSlideCh, &slideReady, &cfg) {
			t.Error("plain text should not be treated as a command")
		}
	})
}

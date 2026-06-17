// Package config 保存运行时配置：应用标识（版本、模型）、ASCII 艺术横幅、提示文本、背景亮度/路径以及颜色主题。默认值复现原始的硬编码值；可选的 JSON 文件可以覆盖它们。
//
// 配置目录（dataDir）在程序启动时自动创建，存放所有参数文件：
//
//	data/
//	  config.json      主配置（覆盖默认值）
//	  api.json         模型 API 配置（用于后续接入大模型）
//	  backgrounds/     壁纸图片
//	    config.json    壁纸轮播/透明度配置
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"strike-core/internal/style"
)

// DataDirName 是存放配置文件的目录名。
const DataDirName = "data"

// buildVersion 通过 -ldflags 在链接时注入。当设置时（非空），它会覆盖默认的 Version，以便发布二进制文件报告其 git 标签。
var buildVersion string

// Config 是供 ui/app 层使用的完全解析的运行时配置。
type Config struct {
	Version      string
	ModelName    string
	SystemPrompt string
	Hint         string
	Brightness   float64
	BgPath       string        // external image override; empty => use embedded asset
	BgInterval   time.Duration // wallpaper slideshow interval (0 = off)
	AsciiArt     []string
	Theme        style.Theme
}

// fileConfig 是 JSON 可解码子集。颜色为十六进制字符串以便文件保持人工可编辑；未设置的字段回退到默认值。
type fileConfig struct {
	Version      *string  `json:"version,omitempty"`
	ModelName    *string  `json:"model_name,omitempty"`
	SystemPrompt *string  `json:"system_prompt,omitempty"`
	Hint         *string  `json:"hint,omitempty"`
	Brightness   *float64 `json:"brightness,omitempty"`
	BgPath       *string  `json:"bg_path,omitempty"`
	BgInterval   *int     `json:"bg_interval,omitempty"` // seconds, 0 = off
	AsciiArt   []string  `json:"ascii_art,omitempty"`
	Theme      *fileTheme `json:"theme,omitempty"`
}

// fileTheme 是 Theme 的 JSON 可解码子集，所有字段均为可选十六进制颜色。
type fileTheme struct {
	ArtLeft         *string `json:"art_left,omitempty"`
	ArtRight        *string `json:"art_right,omitempty"`
	InputAreaBg     *string `json:"input_area_bg,omitempty"`
	InputTextFg     *string `json:"input_text_fg,omitempty"`
	PromptFg        *string `json:"prompt_fg,omitempty"`
	PromptBg        *string `json:"prompt_bg,omitempty"`
	BlockEdgeFg     *string `json:"block_edge_fg,omitempty"`
	BlockEdgeBg     *string `json:"block_edge_bg,omitempty"`
	UserEdgeFg      *string `json:"user_edge_fg,omitempty"`
	AssistantEdgeFg *string `json:"assistant_edge_fg,omitempty"`
	SepFg           *string `json:"sep_fg,omitempty"`
	DimFg           *string `json:"dim_fg,omitempty"`
	PlaceholderFg   *string `json:"placeholder_fg,omitempty"`
	HintFg          *string `json:"hint_fg,omitempty"`
	ModelFg         *string `json:"model_fg,omitempty"`
	LogoDepthBg     *string `json:"logo_depth_bg,omitempty"`
}

// APIConfig 存储模型 API 连接参数，用于后续接入大模型。
type APIConfig struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
}

// DefaultAPI 返回默认的 API 配置。
func DefaultAPI() APIConfig {
	return APIConfig{
		BaseURL: "https://api.minimax.chat/v1",
		Model:   "MiniMaxAI/MiniMax-M2.5",
	}
}

// LoadAPI 从 data/api.json 加载 API 配置，文件不存在时返回默认值。
// STRIKE_API_KEY 环境变量优先级高于配置文件，便于 CI/CD 安全注入。
func LoadAPI(dataDir string) APIConfig {
	cfg := DefaultAPI()
	path := filepath.Join(dataDir, "api.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var fc struct {
			APIKey  *string `json:"api_key,omitempty"`
			BaseURL *string `json:"base_url,omitempty"`
			Model   *string `json:"model,omitempty"`
		}
		if err := json.Unmarshal(raw, &fc); err == nil {
			if fc.APIKey != nil {
				cfg.APIKey = *fc.APIKey
			}
			if fc.BaseURL != nil {
				cfg.BaseURL = *fc.BaseURL
			}
			if fc.Model != nil {
				cfg.Model = *fc.Model
			}
		}
	}
	if envKey := os.Getenv("STRIKE_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	}
	if envURL := os.Getenv("STRIKE_API_URL"); envURL != "" {
		cfg.BaseURL = envURL
	}
	return cfg
}

// EnsureDataDir 创建 dataDir 及其子目录结构（若不存在），然后返回 dataDir 路径。
func EnsureDataDir(workDir string) string {
	dataDir := filepath.Join(workDir, DataDirName)
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(filepath.Join(dataDir, "backgrounds"), 0755)
	return dataDir
}

// Default 返回内置配置（原始硬编码值）。
func Default() Config {
	version := "内部版v26.5"
	if buildVersion != "" {
		version = buildVersion
	}
	return Config{
		Version:      version,
		ModelName:    "MiniMaxAI/MiniMax-M2.5",
		SystemPrompt: "你是一个名为 StrikeCore 的终端 AI 智能体助手。请用中文回答用户的问题。回答简洁有力。",
		Hint:         "↑ 输入内容，Ctrl+C 退出",
		Brightness:   0.35,
		BgPath:       "",
		BgInterval:   60 * time.Second,
		AsciiArt: []string{
			"█▀▀▀ ▀█▀ █▀▀▄ ▀█▀ █ ▄▀ █▀▀▀  █▀▀▀ █▀▀█ █▀▀▄ █▀▀▀",
			"▀▀▀█  █  █▀█   █  █▀▄  █▀▀   █    █  █ █▀█  ▀▀▀█",
			"▀▀▀▀  ▀  ▀  ▀ ▀▀▀ ▀  ▀ ▀▀▀▀  ▀▀▀▀ ▀▀▀▀ ▀  ▀ ▀▀▀▀",
		},
		Theme: style.DefaultTheme(),
	}
}

// Load 读取 JSON 配置文件并将其叠加到 Default() 之上。
// 当 path 为空时会尝试从 dataDir 下的 config.json 加载。
// 路径存在但解析失败则返回错误，以便暴露而非忽略配置错误。
func Load(path string, dataDir string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = filepath.Join(dataDir, "config.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // 没有配置文件也正常
		}
		return cfg, fmt.Errorf("config: read %q: %w", path, err)
	}
	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return cfg, fmt.Errorf("config: parse %q: %w", path, err)
	}
	if err := apply(&cfg, &fc); err != nil {
		return cfg, fmt.Errorf("config: %q: %w", path, err)
	}
	return cfg, nil
}

func apply(cfg *Config, fc *fileConfig) error {
	if fc.Version != nil {
		cfg.Version = *fc.Version
	}
	if fc.ModelName != nil {
		cfg.ModelName = *fc.ModelName
	}
	if fc.SystemPrompt != nil {
		cfg.SystemPrompt = *fc.SystemPrompt
	}
	if fc.Hint != nil {
		cfg.Hint = *fc.Hint
	}
	if fc.Brightness != nil {
		cfg.Brightness = clamp(*fc.Brightness, 0, 1)
	}
	if fc.BgPath != nil {
		cfg.BgPath = *fc.BgPath
	}
	if fc.BgInterval != nil {
		if *fc.BgInterval > 0 {
			cfg.BgInterval = time.Duration(*fc.BgInterval) * time.Second
		} else {
			cfg.BgInterval = 0
		}
	}
	if len(fc.AsciiArt) > 0 {
		cfg.AsciiArt = fc.AsciiArt
	}
	if fc.Theme != nil {
		if err := applyColor(&cfg.Theme.ArtLeft, fc.Theme.ArtLeft); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.ArtRight, fc.Theme.ArtRight); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.InputAreaBg, fc.Theme.InputAreaBg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.InputTextFg, fc.Theme.InputTextFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.PromptFg, fc.Theme.PromptFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.PromptBg, fc.Theme.PromptBg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.BlockEdgeFg, fc.Theme.BlockEdgeFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.BlockEdgeBg, fc.Theme.BlockEdgeBg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.UserEdgeFg, fc.Theme.UserEdgeFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.AssistantEdgeFg, fc.Theme.AssistantEdgeFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.SepFg, fc.Theme.SepFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.DimFg, fc.Theme.DimFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.PlaceholderFg, fc.Theme.PlaceholderFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.HintFg, fc.Theme.HintFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.ModelFg, fc.Theme.ModelFg); err != nil {
			return err
		}
		if err := applyColor(&cfg.Theme.LogoDepthBg, fc.Theme.LogoDepthBg); err != nil {
			return err
		}
	}
	return nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func applyColor(dst *style.Color, hex *string) error {
	if hex == nil {
		return nil
	}
	c, err := style.ParseHex(*hex)
	if err != nil {
		return err
	}
	*dst = c
	return nil
}

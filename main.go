// Command strike-core 是一个跨平台终端 UI 界面，用于 AI 聊天
// 接口。main 函数故意保持精简：解析标志、构建配置、运行。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"strike-core/app"
	"strike-core/internal/config"
)

func main() {
	configPath := flag.String("config", "", "指定 JSON 配置文件路径（可选，默认从 data/config.json 读取）")
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	flag.Parse()

	workDir := resolveWorkDir()
	dataDir := config.EnsureDataDir(workDir)

	cfg, err := config.Load(*configPath, dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	apiCfg := config.LoadAPI(dataDir)

	_ = apiCfg // 供后续接入大模型使用

	if *showVersion {
		fmt.Println(cfg.Version)
		return
	}

	if err := app.Run(cfg, dataDir, workDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveWorkDir() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

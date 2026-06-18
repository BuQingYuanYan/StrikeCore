<p align="center">
  <code>
█▀▀▀ ▀█▀ █▀▀▄ ▀█▀ █ ▄▀ █▀▀▀  █▀▀▀ █▀▀█ █▀▀▄ █▀▀▀<br>
 ▀▀▀█  █  █▀█   █  █▀▄  █▀▀   █    █  █ █▀█  ▀▀▀█<br>
 ▀▀▀▀  ▀  ▀  ▀ ▀▀▀ ▀  ▀ ▀▀▀▀  ▀▀▀▀ ▀▀▀▀ ▀  ▀ ▀▀▀▀
  </code>
</p>

<h3 align="center">StrikeCore</h3>
<p align="center">
  <strong>终端原生 AI 智能体 · 沉浸式 TUI 交互</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white" alt="Go version">
  <img src="https://img.shields.io/badge/License-GPLv3-blue" alt="License">
  <img src="https://img.shields.io/badge/Platform-Windows%20|%20Linux%20|%20macOS-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/Status-Active-brightgreen" alt="Status">
</p>

---

StrikeCore 是一个运行在终端中的 AI 智能体，提供沉浸式全屏 TUI 交互体验。它不是一个聊天网页的终端克隆，而是一个**为终端原生设计的 AI 交互环境**——从像素级渲染到事件循环，全部自研。

> 你看到的不是一个聊天网页，而是一个**有生命的终端智能体**。

---

## 目录

- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [快捷键](#快捷键)
- [鼠标操作](#鼠标操作)
- [架构概览](#架构概览)
- [项目结构](#项目结构)
- [配置说明](#配置说明)
- [跨平台支持](#跨平台支持)
- [测试](#测试)
- [许可](#许可)

---

## 功能特性

### 核心体验

| 特性 | 说明 |
|------|------|
| **AI 对话** | 接入 OpenAI 兼容 API，流式逐字输出 |
| **思考过程可见** | 支持 `reasoning_content`，以暗色独立渲染 |
| **打字机特效** | 100ms 刻度的智能缓冲输出，UI 永远追得上模型，同时保留逐字动画手感 |
| **随时取消回复** | `Ctrl+C` 或双击 `ESC` 中断回复，已生成内容保留并标记「⏹ 已终止」 |
| **上下文感知** | 完整的对话历史管理，支持中断续接、会话恢复 |

### 视觉与交互

| 特性 | 说明 |
|------|------|
| **鲨鱼游泳动画** | 回复时底部显示 `[▰ ▰ 🦈 ▰ ▰]` 动画，表示等待/生成中 |
| **背景图轮播** | 支持目录幻灯片，定时切换壁纸，可独立调节亮度与透明度 |
| **ASCII 横幅** | 可配置的艺术字 logo，与消息流合并滚动 |
| **真彩色渲染** | 基于 VT 转义序列的双缓冲差异刷新引擎，零闪烁 |
| **智能换行编辑** | CJK 宽字符感知的文本输入，支持光标导航与滚动 |
| **鼠标操作** | 滚轮滚动、自绘高亮选中、OSC52 剪贴板复制 |

### 底层能力

| 特性 | 说明 |
|------|------|
| **自研渲染引擎** | 不依赖 ncurses、termbox 等第三方 TUI 库 |
| **双缓冲差异刷新** | 仅输出变化单元格，最大限度减少终端 I/O |
| **跨平台后端** | Windows 原生 kernel32 / Unix termios，统一接口抽象 |
| **单二进制分发** | 零运行时依赖，一个可执行文件即完整工具链 |

---

## 快速开始

### 环境要求

- Go 1.26+
- 支持真彩色与 VT 转义序列的终端：
  - **Windows**: Windows Terminal（推荐）、PowerShell 7+
  - **macOS**: iTerm2、Kitty、Alacritty
  - **Linux**: GNOME Terminal、Konsole、Kitty、Alacritty

### 安装与运行

```bash
# 直接运行（无需编译——Go 自动处理）
go run .

# 或通过 Makefile
make build      # 编译为二进制
make run        # go run .
make build-all  # 交叉编译全平台到 dist/
```

### 命令行选项

```bash
go run .                  # 默认模式
go run . -version         # 打印版本号后退出
go run . -config my.json  # 加载外部配置文件
```

### 配置 API

首次运行前编辑 `data/api.json`：

```json
{
  "api_key": "your-api-key",
  "base_url": "https://api.example.com/v1",
  "model": "your-model-name"
}
```

---

## 快捷键

### AI 回复进行中

| 操作 | 效果 |
|------|------|
| `Ctrl+C` | 取消当前回复，保留已生成内容，末尾追加「⏹ 已终止」标记 |
| `ESC` × 2（600ms 内） | 同上——取消回复 |
| 鼠标滚轮 | 滚动查看历史消息（回复生成期间也可操作） |

取消后输入栏会短暂显示「已终止AI答复」，约 3 秒后或开始输入时自动恢复。

### 空闲时

| 操作 | 效果 |
|------|------|
| `Ctrl+C`（输入栏非空） | 清空输入内容 |
| `Ctrl+C`（输入栏为空） | 提示「再按一次退出」，5 秒内再次按下即退出 |
| 任何其他按键 | 取消退出等待状态 |
| 方向键 ↑ / ↓ | 浏览/切换历史输入 |

---

## 鼠标操作

由于开启了鼠标捕获以支持滚轮，终端原生的文本选中功能被程序接管，因此实现了**程序内自绘选区**，提供一致的跨平台体验。

### 选中与复制

```
┌─────────────────────────────────────────────┐
│  会话区文本 ◄─── 按住左键拖拽 ────► 高亮选区 │
│  输入框文本 ◄─── 按住左键拖拽 ────► 高亮选区 │
│                                            │
│  选中后按 Ctrl+C ──► OSC52 ──► 系统剪贴板   │
└─────────────────────────────────────────────┘
```

- **会话区 + 输入框**均可拖拽选中（支持跨行、CJK 宽字符、反向拖拽、跨区域拖选）
- 选区锚定逻辑行位置，**滚动时高亮跟随内容移动**
- 占位符与提示文案行不可选
- 开始输入或点击不可选区时自动清除选区

### 技术细节

> OSC52 复制依赖终端支持：Windows Terminal、iTerm2、现代 Linux 终端均原生支持。tmux 需 `set -g set-clipboard on`。不支持的环境下复制会无声失败——不会产生错误提示。

### Ctrl+C 优先级

| 上下文 | 行为 |
|--------|------|
| 有选区 | **复制**选区文本到剪贴板 |
| 无选区 + AI 回复中 | **取消**当前 AI 回复 |
| 无选区 + 空闲 | 清空输入 / **退出**确认 |

AI 回复中也**可双击 `ESC`** 取消，与 `Ctrl+C` 等效。

---

## 架构概览

```
┌─────────────────────────────────────────────────────┐
│                   app/app.go                        │
│   事件循环 · 信号处理 · 原始模式生命周期 · 串联一切   │
├─────────────────────────────────────────────────────┤
│               internal/ 分层架构                      │
│                                                     │
│  ┌─────────┐  ┌─────────┐  ┌──────────────────┐    │
│  │ ui/     │  │ screen/ │  │ editor/          │    │
│  │ 布局    │  │ 双缓冲   │  │ 文本编辑器模型    │    │
│  │ 背景    │  │ 差异刷新 │  │ CJK 感知换行     │    │
│  │ 横幅    │  │ VT 输出  │  │ 光标/滚动       │    │
│  │ 消息渲染│  └─────────┘  └──────────────────┘    │
│  └────┬────┘                                       │
│       │                                            │
│  ┌────┴────┐  ┌─────────┐  ┌──────────────────┐    │
│  │ input/  │  │ config/ │  │ terminal/        │    │
│  │ 按键码  │  │ JSON    │  │ 终端接口抽象      │    │
│  │ 鼠标解析│  │ ldflags │  │ Win/Unix 后端    │    │
│  │ 转义序列│  └─────────┘  └──────────────────┘    │
│  └─────────┘                                       │
│                                                     │
│  ┌─────────┐  ┌─────────┐                           │
│  │ style/  │  │ clipboard│                           │
│  │ 颜色    │  │ OSC52    │  ← 叶子包，零依赖         │
│  │ 主题    │  │ 编码     │                           │
│  └─────────┘  └─────────┘                           │
├─────────────────────────────────────────────────────┤
│                    main.go                           │
│            薄入口：参数 → 配置 → app.Run              │
└─────────────────────────────────────────────────────┘
```

### 设计原则

- **有向无环依赖**：`style/`、`input/`、`clipboard/` 为叶子包，`ui/` 组合 `screen/` + `editor/` + `config/` + `style/`，`app/` 串联一切。无循环依赖。
- **平台后端接口化**：唯一与 OS 相关的部分是 `terminal.Terminal` 接口，编译时通过 Go 构建标签选择实现。所有 VT 转义序列（备选屏幕、光标可见性、同步输出、真彩色）均位于平台无关层。
- **纯函数优先**：鼠标解析、选区计算、OSC52 编码均为纯函数，表驱动单测覆盖。

---

## 项目结构

```
├── main.go                   入口：命令行参数 → 配置 → app.Run
├── app/                      事件循环 · 崩溃安全 · 信号生命周期
│   ├── app.go                主循环（~900 行）
│   ├── input.go              通用输入读取调度
│   ├── input_windows.go      Windows ReadConsoleInput 读取器
│   ├── state.go              会话状态结构
│   └── *.go                  跨平台 resize / 其他
├── internal/
│   ├── style/                颜色 · 主题（叶子包）
│   ├── input/                按键码 · 终端转义序列解析 · 鼠标 SGR 协议
│   ├── config/               运行时配置（JSON 合并 + ldflags）
│   ├── screen/               单元格缓冲区 · 双缓冲差异刷新 · VT 输出
│   ├── terminal/             终端接口抽象 · Windows/Unix 后端
│   ├── editor/               纯文本编辑器模型（CJK 换行 · 光标 · 滚动）
│   ├── ui/                   布局 · 背景 · 横幅 · 消息渲染 · 视图
│   └── clipboard/            OSC52 剪贴板编码（叶子包）
├── data/                     运行时生成的配置与资源目录
│   ├── config.json           主配置
│   ├── api.json              API 密钥与模型配置
│   └── backgrounds/          壁纸图片 + 轮播配置
└── Makefile                  构建 · 测试 · 交叉编译
```

---

## 配置说明

### 主配置 `data/config.json`

```json
{
  "model_name": "your-model-name",
  "hint": "↑ 输入内容，Ctrl+C 退出",
  "brightness": 0.35,
  "bg_path": "/path/to/custom.png",
  "bg_interval": 60,
  "ascii_art": ["line1", "line2", "line3"],
  "theme": {
    "art_left": "#60CDFF",
    "art_right": "#0078D4",
    "hint_fg": "#C8A060"
  }
}
```

字段说明：

| 字段 | 类型 | 说明 |
|------|------|------|
| `model_name` | string | AI 模型名称 |
| `hint` | string | 输入栏提示文字 |
| `brightness` | float (0-1) | 背景图片亮度 |
| `bg_path` | string | 自定义背景图路径（覆盖内嵌资源） |
| `bg_interval` | int | 壁纸轮播间隔（秒） |
| `ascii_art` | string[] | 自定义横幅文本行 |
| `theme` | object | 主题颜色覆盖 |

> `bg_path` 加载失败时会优雅降级——先回退到内嵌图片，再回退到纯色背景。

### API 配置 `data/api.json`

```json
{
  "api_key": "your-api-key",
  "base_url": "https://api.example.com/v1",
  "model": "your-model-name"
}
```

### 壁纸配置 `data/backgrounds/config.json`

控制幻灯片行为、气泡背景透明度和图片亮度：

```json
{
  "enabled": true,
  "interval": 60,
  "wallpaper": "",
  "bubble_bg_opacity": 0,
  "brightness": 0.35
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 启用轮播 |
| `interval` | int | 切换间隔（秒） |
| `wallpaper` | string | 指定图片文件（空=文件夹第一张） |
| `bubble_bg_opacity` | float (0-1) | 0=纯色不透明 → 1=完全透明 |
| `brightness` | float (0-1) | 图片亮度 |

> 该文件首次运行时自动生成，也可在运行中通过输入 `/reload` 即时重载。

### 运行时命令

| 命令 | 作用 |
|------|------|
| `/reload` | 重新读取壁纸配置，即时应用透明度、亮度、壁纸切换、轮播开关 |
| `/clear-history` | 清空当前会话历史 |

---

## 跨平台支持

| 能力 | Windows | Unix (Linux/macOS) |
|------|---------|-------------------|
| 终端接口 | kernel32 控制台模式 | `golang.org/x/term` |
| 原始输入 | `ReadConsoleInput` | 标准字节流 |
| 尺寸变化 | 轮询 + `WINDOW_BUFFER_SIZE_EVENT` | `SIGWINCH` 信号 |
| VT 序列 | 全部可移植（?1049h、同步输出、真彩色） | 同左 |
| 鼠标输入 | ReadConsoleInput → SGR 合成 | 原生 VT ?1006h |
| 构建 | `GOOS=windows go build` | `GOOS=linux/darwin go build` |

所有 VT 转义序列（备选屏幕、光标可见性、同步输出、真彩色、鼠标协议）均在平台无关层实现，通过统一的 `terminal.Terminal` 接口与各平台后端交互。

---

## 测试

```bash
make test        # go test ./...
make test-race   # go test -race ./...（需 CGO）
make lint        # go vet + gofmt 检查
```

### 测试覆盖范围

| 包 | 覆盖内容 |
|----|---------|
| `internal/input/` | 按键解析、SGR 鼠标序列（坐标 + 事件类型） |
| `internal/editor/` | 文本编辑操作、CJK 换行、光标滚动 |
| `internal/screen/` | 双缓冲差异刷新（`bytes.Buffer` 断言） |
| `internal/ui/` | 布局几何（黄金测试）、选区命中/抽取、colAtCellX 映射 |
| `internal/clipboard/` | OSC52 编码（base64 正确性、CJK、空串） |
| `internal/config/` | JSON 加载、默认值合并 |
| `app/` | 手柄命令（`/reload`）、核心状态逻辑 |

测试套件在 Windows / Linux / macOS CI 上全平台运行。

---

## 许可

[GNU General Public License v3](LICENSE)

---

<p align="center">
  <sub>StrikeCore · 终端里的 AI 智能体</sub><br>
  <sub>StrikeCore is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.</sub>
</p>

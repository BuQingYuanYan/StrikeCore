# StrikeCore — 终端 AI 智能体

StrikeCore 是一个运行在终端中的 AI 智能体（Agent），提供沉浸式全屏 TUI 交互体验。支持背景图轮播、ASCII 艺术横幅、智能换行文本输入，基于真彩色 VT 转义序列和双缓冲差异渲染引擎。

> 你看到的不是一个聊天网页，而是一个**有生命的终端智能体**。

## 环境要求

- Go 1.26+
- 支持真彩色和 VT 处理的终端（Windows Terminal、现代 Linux/macOS 终端）

## 构建与运行

```sh
make build      # 编译 -> strike-core.exe（Unix 下为 strike-core）
make run        # go run .
make build-all  # 交叉编译 windows/linux/macos 到 dist/
```

直接运行：

```sh
go run .
go run . -version          # 打印版本号后退出
go run . -config my.json   # 加载外部配置文件
```

按 `Ctrl+C`：
- 输入栏有文字 → 清空输入
- 输入栏为空 → 提示"再按一次退出"，5 秒内再按 `Ctrl+C` 才真正退出（按其他键取消）

## 滚动

发送消息后，横幅（logo）与消息气泡合并为一条可滚动的内容流。鼠标滚轮或键盘上下方向键均可滚动：
- 鼠标滚轮：每次滚动 3 行，不受输入栏状态影响
- 上下方向键：输入栏为空时每次滚动 1 行
- 底部分隔线（虚线）以下的输入栏和工作目录行始终固定不动
- 发送新消息时自动滚到底部

鼠标滚轮在 Windows（原生 `ReadConsoleInput`）和 Unix（SGR 鼠标转义序列）上均受支持。

## 项目结构

```
main.go                  薄入口：命令行参数 -> 配置 -> app.Run
data/                    运行时自动创建的配置与资源目录
  config.json            主配置
  api.json               API 配置（供后续接入大模型）
  backgrounds/           壁纸图片 + config.json
app/                     事件循环、原始模式/崩溃/信号安全生命周期
internal/
  style/                 颜色、主题、光标（叶子包）
  input/                 按键码 + 纯终端输入解析器
  config/                运行时配置（JSON 覆盖）+ ldflags 版本号
  screen/                无关后端的单元格缓冲区 + 差异刷新（io.Writer）
  terminal/              终端接口 + Windows/Unix 后端实现
  editor/                纯文本编辑器模型（换行、光标、滚动）
  ui/                    布局、背景（go:embed）、横幅、视图渲染器
```

依赖方向为有向无环图：`style`/`input` 是叶子包，`ui` 组合 `screen`+`editor`+`config`+`style`，`app` 串联一切。

## 跨平台终端

唯一与操作系统相关的部分是 `terminal.Terminal` 接口（`Init`/`Size`/`Out`/`In`），在编译时通过构建标签选择：

- **Windows**（`terminal_windows.go`）：kernel32 控制台模式——启用 VT 处理和原始标准输入。
- **Unix**（`terminal_unix.go`）：`golang.org/x/term`（`MakeRaw` / `GetSize`）。

所有 VT 转义序列（备选屏幕、光标可见性、同步输出、真彩色）都是可移植的，位于 screen/app 层。终端尺寸变化检测：Unix 通过 `SIGWINCH` 信号，Windows 通过 33ms 轮询（外加 `ReadConsoleInput` 实时接收 `WINDOW_BUFFER_SIZE_EVENT`）。

## 配置目录 (`data/`)

程序启动时自动在工作目录下创建 `data/` 文件夹，存放所有可编辑的参数文件：

```
data/
  config.json              主配置（覆盖默认值）
  api.json                 模型 API 配置（用于后续接入大模型）
  backgrounds/             壁纸图片文件夹
    config.json            壁纸轮播/透明度配置
```

### 主配置 `data/config.json`

```json
{
  "version": "内部版v26.5",
  "model_name": "MiniMaxAI/MiniMax-M2.5",
  "hint": "↑ 输入内容，Ctrl+C 退出",
  "brightness": 0.35,
  "bg_path": "/path/to/custom.png",
  "bg_interval": 60,
  "ascii_art": ["line1", "line2", "line3"],
  "theme": { "art_left": "#60CDFF", "art_right": "#0078D4", "hint_fg": "#C8A060" }
}
```

`bg_path` 会覆盖内嵌图片；若加载失败，则回退到内嵌资源（两者都失败时优雅降级为无背景）。也可通过 `-config` 命令行参数指定其他路径。

### API 配置 `data/api.json`

用于后续接入大模型时配置 API 密钥与接口地址：

```json
{
  "api_key": "your-api-key-here",
  "base_url": "https://api.minimax.chat/v1",
  "model": "MiniMaxAI/MiniMax-M2.5"
}
```

### 壁纸配置 `data/backgrounds/config.json`

控制幻灯片行为、气泡背景透明度和图片亮度，无需修改主配置：

```json
{
  "_说明": "enabled=true开启轮播/false关闭轮播   interval=轮播间隔(秒)   wallpaper=指定图片文件名(留空则用文件夹第一张)   bubble_bg_opacity=气泡背景透明度0~1(0=纯色不透明, 数值越大越透明越能看到背景图, 1=完全透明)   brightness=图片亮度0~1(0=最暗, 1=最亮)",
  "enabled": true,
  "interval": 60,
  "wallpaper": "",
  "bubble_bg_opacity": 0,
  "brightness": 0.35
}
```

该文件会在首次运行时自动生成。

## 运行时命令

在输入栏输入 `/reload` 回车即可重新读取 `data/backgrounds/config.json`，
刷新壁纸列表，并即时应用所有更改（透明度、亮度、壁纸切换、轮播开关）。

## 测试

```sh
make test        # go test ./...
make test-race   # go test -race ./...（需要 C 工具链 / CGO）
make lint        # go vet + gofmt 检查
```

纯逻辑均有单元测试覆盖：输入解析器、编辑器操作（包括宽 CJK 字符和滚动）、布局几何计算（黄金测试）、屏幕差异渲染器（通过 `bytes.Buffer` 断言）。CI 在 Windows/Linux/macOS 上运行全套测试。

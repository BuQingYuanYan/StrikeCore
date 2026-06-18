<p align="center">
  <img src="logo.png" alt="StrikeCore" width="480">
</p>

<div align="center">
  <strong>StrikeCore</strong> · 终端原生 AI 智能体<br>
  <sub>不依赖 ncurses、不套 WebView、不妥协性能</sub>
</div>

---

## 快速开始

```bash
git clone https://github.com/BuQingYuanYan/StrikeCore.git && cd StrikeCore
go run .
```

支持 OpenAI 兼容 API 的终端 AI 对话工具。轻量、单二进制、零依赖。

## 功能

- **AI 对话** — 流式 SSE 打字机输出，思考链独立渲染
- **实时 Token 显示** — 流中动态估算 + 完成后替换为厂商精确值，支持单次 / 累计
- **壁纸轮播** — 目录幻灯片，亮度渐入渐出过渡，独立调节亮度 & 透明度
- **中断/续接** — Ctrl+C 或双击 ESC 中断，上下文保持
- **鼠标选中** — 会话区 + 输入框拖拽选区，OSC52 复制到系统剪贴板
- **真彩色渲染** — 自研双缓冲差异刷新，不依赖 ncurses

## 命令

| 命令 | 作用 |
|------|------|
| `/reload` | 重载壁纸配置 |
| `/clear-history` | 清空会话历史 |

## 配置

`data/config.json` — 模型、亮度、壁纸、主题配色  
`data/api.json` — API Key、地址、模型  
`data/backgrounds/config.json` — 壁纸轮播开关、间隔、透明度

首次运行自动生成，支持 `/reload` 即时重载。

## 许可

GNU General Public License v3

## 更新历史

### v0.2.0 — 2026-06

- **实时 Token 显示**：流式输出期间动态估算 token 消耗（ASCII×0.3 + CJK×0.6），完成后替换为厂商 API 返回的精确值，支持单次（🚀 N）与累计（∑ N）模式
- **Logo 美化**：更新 ASCII 横幅，末两行连续背景铺满，模型名称独立显示
- **壁纸淡入淡出**：亮度动画交叉淡入淡出过渡，替代硬切换
- **输入区隔断**：气泡区与输入栏之间保留一行壁纸透出行，防止视觉混淆
- **滚动优化**：AI 响应期间鼠标滚轮不再跳回底部
- **中断提示优化**：统一风格，缩短重复行距
- **Token 统计精度**：Prompt 部分仅计算对话内容，排除固定系统提示词

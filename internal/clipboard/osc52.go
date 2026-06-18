// Package clipboard 提供基于 OSC52 转义序列的剪贴板写入，纯文本序列由终端解释。
package clipboard

import "encoding/base64"

// Encode 编码 OSC52 序列：\x1b]52;c;<base64>\x07。
func Encode(text string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + enc + "\x07"
}

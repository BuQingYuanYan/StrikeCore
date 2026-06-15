package ui

import "github.com/mattn/go-runewidth"

// artData 持有根据配置的 ASCII 艺术字预计算的横幅几何信息。
// 它取代了旧 main.go 中的包初始化全局变量。
type artData struct {
	texts []string
	leftW []int // display width of the left (lighter) color segment per row
	width int   // max banner width
}

const artMid = 29 // 左侧颜色切换为右侧颜色的符文索引

func buildArt(lines []string) artData {
	a := artData{
		texts: make([]string, len(lines)),
		leftW: make([]int, len(lines)),
	}
	for i, line := range lines {
		runes := []rune(line)
		split := artMid
		if split > len(runes) {
			split = len(runes)
		}
		a.texts[i] = line
		a.leftW[i] = runewidth.StringWidth(string(runes[:split]))
		if w := runewidth.StringWidth(line); w > a.width {
			a.width = w
		}
	}
	return a
}

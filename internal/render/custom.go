package render

// 本文件实现参考图所需的新能力：双色渐变背景、自定义图标（PNG/JPG）、
// 圆角玻璃质感图标容器、文字阴影。与 render.go 的矢量图标系统并存。

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // 注册 JPEG 解码器
	_ "image/png"  // 注册 PNG 解码器
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	xd "golang.org/x/image/draw"
)

// customIconDir 自定义图标存放目录（由应用层启动时设置）。
var customIconDir string

// SetCustomIconDir 设置自定义图标目录。
func SetCustomIconDir(dir string) { customIconDir = dir }

// GetCustomIconDir 返回自定义图标目录。
func GetCustomIconDir() string { return customIconDir }

// IsCustomIcon 判断图标标识是否为自定义图标（以 custom: 开头）。
func IsCustomIcon(kind string) bool { return strings.HasPrefix(kind, "custom:") }

// CustomIconName 从标识中提取文件名。
func CustomIconName(kind string) string { return strings.TrimPrefix(kind, "custom:") }

// lerpColor 在两色间线性插值，t∈[0,1]。
func lerpColor(c1, c2 color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float64(c1.R) + (float64(c2.R)-float64(c1.R))*t),
		G: uint8(float64(c1.G) + (float64(c2.G)-float64(c1.G))*t),
		B: uint8(float64(c1.B) + (float64(c2.B)-float64(c1.B))*t),
		A: 255,
	}
}

// drawGradient 用两色填充画布，生成线性渐变背景。
// dir: 0=左上到右下, 1=上到下, 2=左到右, 3=右上到左下。
func drawGradient(dc *gg.Context, c1, c2 color.RGBA, dir int) {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			fx, fy := float64(x)/float64(W-1), float64(y)/float64(H-1)
			var t float64
			switch dir {
			case 1: // 上到下
				t = fy
			case 2: // 左到右
				t = fx
			case 3: // 右上到左下
				t = (1-fx + fy) / 2
			default: // 左上到右下
				t = (fx + fy) / 2
			}
			img.SetRGBA(x, y, lerpColor(c1, c2, t))
		}
	}
	dc.DrawImage(img, 0, 0)
}

// loadCustomImage 读取自定义图标文件（PNG/JPG/GIF）。
func loadCustomImage(kind string) (image.Image, error) {
	name := CustomIconName(kind)
	// 安全校验：禁止路径穿越
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return nil, errors.New("非法图标文件名")
	}
	if customIconDir == "" {
		return nil, errors.New("自定义图标目录未初始化")
	}
	path := filepath.Join(customIconDir, name)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取图标失败: %w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码图标失败(需 PNG/JPG): %w", err)
	}
	return img, nil
}

// drawCustomIcon 绘制自定义图标。
// monochrome=true 时转为单色剪影（保留 alpha，用 col 填充）；false 保留原色。
// frame=true 时先画圆角玻璃白卡与投影，再画图标（参考图那种 App 图标容器）。
func drawCustomIcon(dc *gg.Context, kind string, cx, cy float64, scale float64, col color.RGBA, monochrome, frame bool) error {
	img, err := loadCustomImage(kind)
	if err != nil {
		return err
	}
	// 统一边界框：图标最大边 = 2*I（与内置图标同量级）
	maxDim := 2 * 72 * SCALE * scale
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return errors.New("图标尺寸无效")
	}
	side := iw
	if ih > side {
		side = ih
	}
	sc := maxDim / float64(side)
	nw, nh := float64(iw)*sc, float64(ih)*sc

	// 圆角玻璃质感容器 + 投影
	if frame {
		pad := 0.16 * maxDim
		box := maxDim + pad*2
		radius := 0.16 * box
		bx, by := cx-box/2, cy-box/2
		// 投影（模糊矩形：叠几层半透明圆角矩形）
		for i := 5; i >= 1; i-- {
			sp := float64(i) * 1.6 * scale
			dc.SetRGBA(0.08, 0.10, 0.20, 0.035)
			dc.DrawRoundedRectangle(bx+sp*0.3, by+sp, box, box, radius)
			dc.Fill()
		}
		// 玻璃白卡主体
		dc.SetRGBA(1, 1, 1, 0.86)
		dc.DrawRoundedRectangle(bx, by, box, box, radius)
		dc.Fill()
		// 顶部高光描边
		dc.SetRGBA(1, 1, 1, 0.75)
		dc.SetLineWidth(1.6 * scale)
		dc.DrawRoundedRectangle(bx+1, by+1, box-2, box-2, radius)
		dc.Stroke()
	}

	// 缩放图片
	resized := image.NewRGBA(image.Rect(0, 0, int(nw), int(nh)))
	xd.CatmullRom.Scale(resized, resized.Bounds(), img, b, xd.Over, nil)

	// 单色剪影：保留 alpha，用 col 填充
	drawImg := image.Image(resized)
	if monochrome {
		sil := image.NewRGBA(resized.Bounds())
		for y := 0; y < resized.Bounds().Dy(); y++ {
			for x := 0; x < resized.Bounds().Dx(); x++ {
				_, _, _, a := resized.At(x, y).RGBA()
				if a > 0 {
					sil.SetRGBA(x, y, color.RGBA{R: col.R, G: col.G, B: col.B, A: uint8(a >> 8)})
				}
			}
		}
		drawImg = sil
	}

	dc.DrawImage(drawImg, int(cx-nw/2), int(cy-nh/2))
	return nil
}

// drawTextShadow 画文字的柔和投影（在文字本体绘制前调用）。
// 用多层小偏移叠加模拟柔和的加粗投影，避免硬边描边感。
func drawTextShadow(dc *gg.Context, s string, x, y float64, offset float64) {
	layers := []struct {
		ox, oy float64
		a      float64
	}{
		{0.5, 0.5, 0.10}, {1.0, 1.0, 0.10}, {1.5, 1.5, 0.08},
		{0.5, 1.2, 0.08}, {1.2, 0.5, 0.08},
	}
	for _, l := range layers {
		dc.SetRGBA(0.04, 0.06, 0.22, l.a)
		dc.DrawStringAnchored(s, x+offset*l.ox, y+offset*l.oy, 0.5, 0.5)
	}
}

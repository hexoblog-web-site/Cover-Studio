// Package render 负责封面绘制：纯色背景 + 居中图标 + 左右圆润加粗文案。
// 移植自 Hexo 站点封面生成脚本（Python/PIL），坐标系与图标保持完全一致。
package render

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

//go:embed STXIHEI.TTF
var fontFS embed.FS

// rad 角度转弧度（等价于其他语言的 math.radians）。
func rad(deg float64) float64 { return deg * math.Pi / 180 }

const (
	W      = 1000
	H      = 500
	SCALE  = 1.6
	SW     = 2 // 同色描边合成加粗（避免圆体发糊）
	FS     = 128
)

var (
	fontOnce sync.Once
	fontFace *truetype.Font
	fontErr  error
)

func loadFont() (*truetype.Font, error) {
	fontOnce.Do(func() {
		b, err := fontFS.ReadFile("STXIHEI.TTF")
		if err != nil {
			fontErr = err
			return
		}
		fontFace, fontErr = truetype.Parse(b)
	})
	return fontFace, fontErr
}

// hex2rgb 解析 #RRGGBB
func hex2rgb(h string) (color.RGBA, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "#")
	if len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("颜色格式无效: %q (需 #RRGGBB)", h)
	}
	var r, g, b int
	if _, err := fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{}, fmt.Errorf("颜色解析失败: %q", h)
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}

// IconNames 返回全部可用图标的英文标识，供前端下拉框使用。
func IconNames() []string {
	return []string{
		"hexagon", "rocket", "lock", "cpu", "cloud", "trash", "chart", "db",
		"server", "home", "book", "sparkle", "star", "puzzle", "gear", "chain",
		"sidebar", "shield", "tag", "clock", "file", "envelope", "sliders", "arrowcircle",
		"folder", "comment", "palette", "dice", "calendar", "globe", "images", "layout",
		"apps", "hourglass", "megaphone", "users", "heart",
	}
}

// Params 绘图参数。颜色留空或数值 <=0 时回退默认值（保持向后兼容）。
type Params struct {
	Left      string  // 左侧文案
	Right     string  // 右侧文案
	BG        string  // 背景色 #RRGGBB
	Icon      string  // 图标标识
	LX, RX    float64 // 左右文字锚点 x（默认 260 / 740）
	FontColor string  // 文字颜色 #RRGGBB（默认白色）
	FontSize  float64 // 字号（默认 FS=128）
	IconColor string  // 图标颜色 #RRGGBB（默认白色）
	IconScale float64 // 图标缩放倍数（默认 1.0）
	Glow      float64 // 图标光晕强度 0~1（<=0 关闭，默认 0.55）
	IconOnTop bool    // true=图标盖住文字；false=文字盖住图标（默认）
	// 参考图风格新增字段
	BG2         string  // 渐变第二色 #RRGGBB（空=纯色背景）
	GradDir     int     // 渐变方向 0=左上→右下 1=上→下 2=左→右 3=右上→左下
	IconFrame   bool    // 自定义图标是否加圆角玻璃容器
	MonoIcon    bool    // 自定义图标是否转单色剪影（false=保留原色）
	TextShadow  bool    // 文字是否加柔和投影
}

// resolveColor 解析颜色字符串；空串返回默认色。
func resolveColor(h string, def color.RGBA) (color.RGBA, error) {
	if strings.TrimSpace(h) == "" {
		return def, nil
	}
	return hex2rgb(h)
}

// Render 生成封面 PNG 字节。
func Render(p Params) ([]byte, error) {
	bgc, err := hex2rgb(p.BG)
	if err != nil {
		return nil, err
	}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	fontCol, err := resolveColor(p.FontColor, white)
	if err != nil {
		return nil, fmt.Errorf("文字颜色: %w", err)
	}
	iconCol, err := resolveColor(p.IconColor, white)
	if err != nil {
		return nil, fmt.Errorf("图标颜色: %w", err)
	}
	fs := p.FontSize
	if fs <= 0 {
		fs = FS
	}
	is := p.IconScale
	if is <= 0 {
		is = 1.0
	}
	glow := p.Glow
	if glow < 0 {
		glow = 0
	}
	if glow > 1 {
		glow = 1
	}
	f, err := loadFont()
	if err != nil {
		return nil, err
	}
	dc := gg.NewContext(W, H)
	// 背景：有 BG2 则双色渐变，否则纯色
	if strings.TrimSpace(p.BG2) != "" {
		bg2, e := hex2rgb(p.BG2)
		if e != nil {
			return nil, fmt.Errorf("渐变第二色: %w", e)
		}
		drawGradient(dc, bgc, bg2, p.GradDir)
	} else {
		dc.SetColor(bgc)
		dc.Clear()
	}

	cx, cy := float64(W)/2, float64(H)/2

	// 文案绘制（圆润加粗），封装以便按叠加顺序调用。
	// halo=true 时先用背景色画一圈稍粗的字，在图标线条上"挖"出清晰隔断，
	// 让"文字在上"即使与图标同色（如白字+白图标）也能明显看出文字压住图标。
	drawText := func(halo bool) {
		face := truetype.NewFace(f, &truetype.Options{Size: fs, DPI: 72})
		dc.SetFontFace(face)
		lx, rx := p.LX, p.RX
		if lx == 0 {
			lx = 260
		}
		if rx == 0 {
			rx = 740
		}
		// 描边随字号等比缩放，保持加粗比例一致
		sw := SW * fs / FS
		// 中文字体 mm 锚点相对视觉中心偏下，且与字号成正比（实测约 0.22×字号）。
		// 按字号上移补偿，使文字视觉质心落在画面中线；阴影/halo/本体共用同一基准 y。
		ty := cy - fs*0.22
		if p.TextShadow {
			sh := fs * 0.035
			drawTextShadow(dc, p.Left, lx, ty, sh)
			drawTextShadow(dc, p.Right, rx, ty, sh)
		}
		if halo {
			dc.SetColor(bgc)
			hw := sw + fs*0.022 // 背景色描边略宽于字，形成隔断（随字号缩放）
			drawBoldAnchored(dc, p.Left, lx, ty, hw)
			drawBoldAnchored(dc, p.Right, rx, ty, hw)
		}
		dc.SetColor(fontCol)
		drawBoldAnchored(dc, p.Left, lx, ty, sw)
		drawBoldAnchored(dc, p.Right, rx, ty, sw)
	}
	// 光晕始终作为图标的一部分紧随其后，保持与图标同层
	drawIconWithGlow := func() error {
		if glow > 0 {
			drawGlow(dc, p.Icon, cx, cy, iconCol, is, glow)
		}
		if IsCustomIcon(p.Icon) {
			return drawCustomIcon(dc, p.Icon, cx, cy, is, iconCol, p.MonoIcon, p.IconFrame)
		}
		drawIcon(p.Icon, dc, cx, cy, iconCol, bgc, is)
		return nil
	}

	// 叠加顺序：后绘制者盖在先绘制者之上。
	// 文字在上时启用背景色 halo，让文字在图标上挖出隔断，同色也能看清层级。
	if p.IconOnTop {
		drawText(false)
		if err := drawIconWithGlow(); err != nil {
			return nil, err
		}
	} else {
		if err := drawIconWithGlow(); err != nil {
			return nil, err
		}
		drawText(true)
	}

	// 用 BestSpeed 编码：封面是纯色/渐变大色块，无损但压缩更快，CPU 与耗时都明显下降
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawBoldAnchored 用同色多重描边模拟加粗，中心对齐。
func drawBoldAnchored(dc *gg.Context, s string, x, y, sw float64) {
	// 先画一圈偏移，再画中心，形成柔和加粗（同色描边，非黑边）
	for _, o := range [][2]float64{{-sw, 0}, {sw, 0}, {0, -sw}, {0, sw}} {
		dc.DrawStringAnchored(s, x+o[0], y+o[1], 0.5, 0.5)
	}
	dc.DrawStringAnchored(s, x, y, 0.5, 0.5)
}

// ---------- 图标光晕 ----------

// drawGlow 为图标绘制柔和光晕。
// 做法：在透明离屏画布上用纯色画图标 → 取其 alpha 通道做多次 box blur 近似高斯 →
// 用图标色给模糊后的遮罩着色并按 strength 缩放透明度 → 叠到主画布（在实体图标之下）。
// 优点：对所有 37 个图标通用，无需改动各自的绘制代码；光晕严格贴合图标轮廓。
func drawGlow(dc *gg.Context, kind string, cx, cy float64, col color.RGBA, scale, strength float64) {
	// 只在图标附近的小方框里计算模糊，避免每次都遍历整张 1000×500 画布。
	// 光晕在到达方框边缘前已衰减到接近 0，视觉结果与全图模糊一致。
	iconR := 72 * SCALE * scale          // 图标最大边界半径
	pad := 16*3 + 8                      // 模糊半径×迭代次数 + 余量
	R := int(math.Ceil(iconR)) + pad
	size := R * 2
	lc := float64(R) // 图标在局部画布中的中心
	layer := gg.NewContext(size, size)
	if IsCustomIcon(kind) {
		_ = drawCustomIcon(layer, kind, lc, lc, scale, col, true, false)
	} else {
		drawIcon(kind, layer, lc, lc, col, color.RGBA{0, 0, 0, 0}, scale)
	}

	mask := blurAlpha(layer.Image(), 16, 3) // 半径 16px、3 次迭代近似高斯
	glow := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := float64(mask.AlphaAt(x, y).A) * strength
			v := uint8(a)
			if v > 0 {
				glow.SetRGBA(x, y, color.RGBA{R: col.R, G: col.G, B: col.B, A: v})
			}
		}
	}
	dc.DrawImage(glow, int(cx)-R, int(cy)-R)
}

// blurAlpha 对图像 alpha 通道做可分离 box blur，passes 次迭代近似高斯模糊。
func blurAlpha(src image.Image, r, passes int) *image.Alpha {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	cur := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := src.At(x+b.Min.X, y+b.Min.Y).RGBA()
			cur.SetAlpha(x, y, color.Alpha{A: uint8(a >> 8)})
		}
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	win := 2*r + 1
	for p := 0; p < passes; p++ {
		tmp := image.NewAlpha(image.Rect(0, 0, w, h))
		// 水平方向
		for y := 0; y < h; y++ {
			sum := 0
			for k := -r; k <= r; k++ {
				sum += int(cur.AlphaAt(clamp(k, 0, w-1), y).A)
			}
			for x := 0; x < w; x++ {
				tmp.SetAlpha(x, y, color.Alpha{A: uint8(sum / win)})
				sum += int(cur.AlphaAt(clamp(x+r+1, 0, w-1), y).A) - int(cur.AlphaAt(clamp(x-r, 0, w-1), y).A)
			}
		}
		// 垂直方向
		for x := 0; x < w; x++ {
			sum := 0
			for k := -r; k <= r; k++ {
				sum += int(tmp.AlphaAt(x, clamp(k, 0, h-1)).A)
			}
			for y := 0; y < h; y++ {
				cur.SetAlpha(x, y, color.Alpha{A: uint8(sum / win)})
				sum += int(tmp.AlphaAt(x, clamp(y+r+1, 0, h-1)).A) - int(tmp.AlphaAt(x, clamp(y-r, 0, h-1)).A)
			}
		}
	}
	return cur
}

// ---------- 绘图辅助 ----------

type ctx struct {
	dc   *gg.Context
	cx   float64
	cy   float64
	I    float64
	col  color.RGBA
	bgc  color.RGBA
	lw   float64
}

func newCtx(dc *gg.Context, cx, cy float64, col, bgc color.RGBA, iconScale float64) *ctx {
	return &ctx{
		dc: dc, cx: cx, cy: cy, col: col, bgc: bgc,
		I:  72 * SCALE * iconScale, // 统一边界框随缩放等比放大
		lw: 13 * SCALE * iconScale, // 线宽同步放大，保持比例
	}
}

func (c *ctx) P(u, v float64) (float64, float64) {
	return c.cx + u*c.I, c.cy + v*c.I
}

func (c *ctx) setFill(col color.RGBA) { c.dc.SetColor(col) }
func (c *ctx) fill()                  { c.dc.Fill() }
func (c *ctx) stroke()                { c.dc.Stroke() }

func (c *ctx) disc(u, v, r float64, col color.RGBA) {
	x, y := c.P(u, v)
	c.setFill(col)
	c.dc.DrawCircle(x, y, r*c.I)
	c.fill()
}

func (c *ctx) ellipse(u0, v0, u1, v1 float64, col color.RGBA) {
	x0, y0 := c.P(u0, v0)
	x1, y1 := c.P(u1, v1)
	c.setFill(col)
	c.dc.DrawEllipse((x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2)
	c.fill()
}

func (c *ctx) ellipseOutline(u0, v0, u1, v1 float64, col color.RGBA, lw float64) {
	x0, y0 := c.P(u0, v0)
	x1, y1 := c.P(u1, v1)
	c.dc.SetColor(col)
	c.dc.SetLineWidth(lw)
	c.dc.DrawEllipse((x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2)
	c.stroke()
}

func (c *ctx) ring(u, v, r float64, col color.RGBA, lw float64) {
	x, y := c.P(u, v)
	c.dc.SetColor(col)
	c.dc.SetLineWidth(lw)
	c.dc.DrawCircle(x, y, r*c.I)
	c.stroke()
}

func (c *ctx) rrect(u, v, w, h, rad float64, col color.RGBA, outline bool, lw float64) {
	x, y := c.P(u, v)
	c.dc.SetColor(col)
	c.dc.SetLineWidth(lw)
	c.dc.DrawRoundedRectangle(x-w*c.I/2, y-h*c.I/2, w*c.I, h*c.I, rad*c.I)
	if outline {
		c.stroke()
	} else {
		c.fill()
	}
}

func (c *ctx) rect(u0, v0, u1, v1 float64, col color.RGBA) {
	x0, y0 := c.P(u0, v0)
	x1, y1 := c.P(u1, v1)
	c.setFill(col)
	c.dc.DrawRectangle(x0, y0, x1-x0, y1-y0)
	c.fill()
}

func (c *ctx) line(u0, v0, u1, v1 float64, col color.RGBA, lw float64) {
	x0, y0 := c.P(u0, v0)
	x1, y1 := c.P(u1, v1)
	c.dc.SetColor(col)
	c.dc.SetLineWidth(lw)
	c.dc.MoveTo(x0, y0)
	c.dc.LineTo(x1, y1)
	c.stroke()
}

func (c *ctx) polygon(points [][2]float64, col color.RGBA) {
	c.setFill(col)
	c.dc.MoveTo(c.cx+points[0][0]*c.I, c.cy+points[0][1]*c.I)
	for _, p := range points[1:] {
		c.dc.LineTo(c.cx+p[0]*c.I, c.cy+p[1]*c.I)
	}
	c.dc.ClosePath()
	c.fill()
}

func (c *ctx) hexagon(r float64, col color.RGBA, lw float64) {
	c.dc.SetColor(col)
	c.dc.SetLineWidth(lw)
	c.dc.MoveTo(c.cx+r*c.I, c.cy)
	for i := 1; i < 6; i++ {
		a := rad(float64(60 * i))
		c.dc.LineTo(c.cx+r*c.I*math.Cos(a), c.cy+r*c.I*math.Sin(a))
	}
	c.dc.ClosePath()
	c.stroke()
}

// drawIcon 根据类型绘制居中图标（归一化坐标，最远点落在 ±I 内）。
// scale 为缩放倍数，作用于边界框 I 与所有线宽，保证整体等比放大/缩小。
func drawIcon(kind string, dc *gg.Context, cx, cy float64, col, bgc color.RGBA, scale float64) {
	c := newCtx(dc, cx, cy, col, bgc, scale)
	w := func(x float64) float64 { return x * SCALE * scale }
	lw := c.lw

	switch kind {
	case "hexagon":
		c.hexagon(0.92, col, w(16))
		c.hexagon(0.46, col, w(10))
		c.disc(0, 0, 0.10, col)
	case "rocket":
		c.polygon([][2]float64{{0, -0.80}, {-0.30, -0.10}, {-0.16, 0.46}, {0.16, 0.46}, {0.30, -0.10}}, col)
		c.disc(0, -0.30, 0.15, bgc)
		c.polygon([][2]float64{{-0.30, -0.10}, {-0.48, 0.22}, {-0.16, 0.20}}, col)
		c.polygon([][2]float64{{0.30, -0.10}, {0.48, 0.22}, {0.16, 0.20}}, col)
		c.rrect(0, 0.48, 0.34, 0.24, 0.10, col, false, lw)
	case "lock":
		c.rrect(0, 0.28, 0.78, 0.58, 0.12, col, false, lw)
		c.dc.SetColor(col)
		c.dc.SetLineWidth(lw)
		c.dc.DrawArc(cx, cy-0.02*c.I, 0.26*c.I, math.Pi, 2*math.Pi)
		c.stroke()
		c.disc(0, 0.30, 0.10, bgc)
		c.rect(-0.035, 0.30, 0.035, 0.46, bgc)
	case "cpu":
		c.rrect(0, 0, 0.60, 0.60, 0.08, col, false, lw)
		for _, i := range []float64{-0.22, 0, 0.22} {
			c.rect(-0.42, i-0.05, -0.31, i+0.05, col)
			c.rect(0.31, i-0.05, 0.42, i+0.05, col)
			c.rect(i-0.05, -0.42, i+0.05, -0.31, col)
			c.rect(i-0.05, 0.31, i+0.05, 0.42, col)
		}
		c.disc(0, 0, 0.17, bgc)
		c.disc(0, 0, 0.09, col)
	case "cloud":
		c.disc(-0.30, -0.05, 0.30, col)
		c.disc(0, -0.30, 0.34, col)
		c.disc(0.32, -0.02, 0.28, col)
		c.rrect(0, 0.18, 0.78, 0.42, 0.20, col, false, lw)
	case "trash":
		c.rrect(0, -0.40, 0.80, 0.12, 0.05, col, false, lw)
		c.rect(-0.10, -0.52, 0.10, -0.40, col)
		c.rrect(0, 0.10, 0.66, 0.82, 0.10, col, false, lw)
		for _, dx := range []float64{-0.18, 0, 0.18} {
			c.rect(dx-0.04, -0.12, dx+0.04, 0.42, bgc)
		}
	case "chart":
		c.line(-0.58, 0.58, 0.58, 0.58, col, w(6))
		c.line(-0.58, 0.58, -0.58, -0.58, col, w(6))
		c.rrect(-0.32, 0.22, 0.24, 0.36, 0.04, col, false, lw)
		c.rrect(0, 0.04, 0.24, 0.54, 0.04, col, false, lw)
		c.rrect(0.32, -0.18, 0.24, 0.76, 0.04, col, false, lw)
		c.polygon([][2]float64{{0.32, -0.62}, {0.16, -0.42}, {0.28, -0.42}, {0.28, -0.22}, {0.36, -0.22}, {0.36, -0.42}, {0.48, -0.42}}, col)
	case "db":
		c.rect(-0.52, -0.18, 0.52, 0.40, col)
		c.ellipse(-0.52, -0.36, 0.52, 0.00, col)
		c.ellipse(-0.52, 0.00, 0.52, 0.36, col)
		c.line(-0.52, 0, 0.52, 0, bgc, w(4))
	case "server":
		c.rrect(0, -0.18, 0.92, 0.50, 0.10, col, true, lw)
		c.rrect(0, 0.45, 0.30, 0.14, 0.05, col, false, lw)
		c.line(-0.18, 0.28, 0.18, 0.28, col, w(8))
		c.disc(0, -0.02, 0.07, col)
	case "home":
		c.polygon([][2]float64{{0, -0.85}, {-0.46, -0.10}, {0.46, -0.10}}, col)
		c.rrect(0, 0.30, 0.64, 0.55, 0.08, col, false, lw)
		c.rect(-0.10, 0.55, 0.10, 0.70, bgc)
	case "book":
		c.rrect(-0.04, 0, 0.82, 0.92, 0.08, col, false, lw)
		c.rect(-0.24, -0.42, 0.10, 0.42, bgc)
		for _, oy := range []float64{-0.24, 0, 0.24} {
			c.line(-0.34, oy, -0.12, oy, bgc, w(5))
		}
	case "sparkle":
		c.polygon([][2]float64{{0, -0.88}, {0.14, -0.14}, {0.88, 0}, {0.14, 0.14}, {0, 0.88}, {-0.14, 0.14}, {-0.88, 0}, {-0.14, -0.14}}, col)
	case "star":
		pts := [][2]float64{}
		for i := 0; i < 10; i++ {
			a := rad(-90 + float64(i*36))
			r := 1.0
			if i%2 != 0 {
				r = 0.45
			}
			pts = append(pts, [2]float64{r * math.Cos(a), r * math.Sin(a)})
		}
		c.polygon(pts, col)
	case "puzzle":
		c.rrect(0, 0, 0.90, 0.90, 0.16, col, false, lw)
		c.disc(0, -0.78, 0.16, col)
		c.disc(0.78, 0, 0.16, col)
		c.disc(0, 0.18, 0.14, bgc)
		c.disc(0, -0.52, 0.12, bgc)
		c.disc(0.52, 0, 0.12, bgc)
	case "gear":
		r := 0.55
		for i := 0; i < 8; i++ {
			a := rad(float64(i * 45))
			c.dc.SetColor(col)
			c.dc.SetLineWidth(lw)
			x0, y0 := c.cx+r*c.I*math.Cos(a), c.cy+r*c.I*math.Sin(a)
			x1, y1 := c.cx+(r+0.16)*c.I*math.Cos(a), c.cy+(r+0.16)*c.I*math.Sin(a)
			c.dc.MoveTo(x0, y0)
			c.dc.LineTo(x1, y1)
			c.stroke()
		}
		c.disc(0, 0, 0.55, col)
		c.disc(0, 0, 0.22, bgc)
	case "chain":
		c.rrect(-0.20, -0.24, 0.54, 0.42, 0.21, col, false, lw)
		c.rrect(0.20, 0.24, 0.54, 0.42, 0.21, col, false, lw)
		c.rrect(-0.20, -0.24, 0.30, 0.20, 0.10, bgc, false, lw)
		c.rrect(0.20, 0.24, 0.30, 0.20, 0.10, bgc, false, lw)
	case "sidebar":
		c.rrect(0, 0, 0.88, 0.95, 0.10, col, true, lw)
		c.rrect(-0.20, 0, 0.20, 0.80, 0.05, col, false, lw)
		for _, oy := range []float64{-0.24, 0, 0.24} {
			c.line(0.02, oy-0.10, 0.34, oy-0.10, col, w(7))
			c.line(0.02, oy+0.10, 0.34, oy+0.10, col, w(7))
		}
	case "shield":
		c.polygon([][2]float64{{0, -0.88}, {0.44, -0.62}, {0.44, 0.16}, {0, 0.88}, {-0.44, 0.16}, {-0.44, -0.62}}, col)
		c.line(-0.22, 0.10, -0.06, 0.38, bgc, lw)
		c.line(-0.06, 0.38, 0.26, -0.36, bgc, lw)
	case "tag":
		c.rrect(-0.06, 0, 0.88, 0.56, 0.10, col, false, lw)
		c.disc(-0.22, 0, 0.09, bgc)
		c.polygon([][2]float64{{0.38, -0.28}, {0.62, 0}, {0.38, 0.28}}, col)
	case "clock":
		c.ring(0, 0, 0.58, col, lw)
		c.line(0, 0, 0, -0.38, col, w(8))
		c.line(0, 0, 0.26, 0.10, col, w(8))
		c.disc(0, 0, 0.07, col)
	case "file":
		c.rrect(0, 0, 0.80, 0.95, 0.10, col, false, lw)
		c.polygon([][2]float64{{0.14, -0.95}, {0.40, -0.62}, {0.14, -0.62}}, bgc)
		for _, oy := range []float64{-0.20, 0.02, 0.24} {
			c.line(-0.24, oy, 0.24, oy, bgc, w(7))
		}
	case "envelope":
		c.rrect(0, 0, 0.92, 0.64, 0.10, col, false, lw)
		c.polygon([][2]float64{{-0.46, -0.32}, {0, -0.02}, {0.46, -0.32}}, bgc)
		c.polygon([][2]float64{{-0.46, 0.32}, {0, 0.04}, {0.46, 0.32}}, bgc)
	case "sliders":
		c.rrect(0, 0, 0.72, 0.88, 0.10, col, false, lw)
		for _, oy := range []float64{-0.26, 0, 0.26} {
			c.rect(-0.28, oy-0.05, 0.28, oy+0.05, bgc)
			c.disc(0.08, oy, 0.11, col)
			c.disc(0.08, oy, 0.06, bgc)
		}
	case "arrowcircle":
		c.ring(0, 0, 0.80, col, lw)
		c.polygon([][2]float64{{0, -0.64}, {-0.26, -0.32}, {-0.10, -0.32}, {-0.10, 0.20}, {0.10, 0.20}, {0.10, -0.32}, {0.26, -0.32}}, col)
	case "folder":
		c.rrect(0, 0.04, 0.90, 0.62, 0.08, col, false, lw)
		c.polygon([][2]float64{{-0.44, -0.24}, {-0.20, -0.24}, {-0.12, -0.10}, {0.10, -0.10}, {0.18, -0.24}, {0.44, -0.24}}, col)
	case "comment":
		c.rrect(0, -0.10, 0.92, 0.74, 0.16, col, false, lw)
		c.polygon([][2]float64{{-0.20, 0.22}, {-0.20, 0.46}, {0.04, 0.22}}, col)
	case "palette":
		c.ellipse(-0.72, -0.60, 0.72, 0.58, col)
		// 拇指缺口用背景色圆近似
		c.disc(0.34, 0.44, 0.20, bgc)
		for _, d := range [][3]float64{{-0.30, -0.16, 0.12}, {-0.02, -0.28, 0.10}, {0.28, -0.12, 0.11}, {0.12, 0.16, 0.10}} {
			c.disc(d[0], d[1], d[2], bgc)
		}
	case "dice":
		c.rrect(0, 0, 0.86, 0.86, 0.18, col, false, lw)
		for _, d := range [][2]float64{{-0.24, -0.24}, {-0.24, 0.24}, {0.24, -0.24}, {0.24, 0.24}, {0, 0}} {
			c.disc(d[0], d[1], 0.09, bgc)
		}
	case "calendar":
		c.rrect(0, 0, 0.92, 0.80, 0.10, col, true, lw)
		c.rect(-0.26, -0.62, -0.16, -0.45, col)
		c.rect(0.16, -0.62, 0.26, -0.45, col)
		c.line(-0.46, -0.10, 0.46, -0.10, col, w(6))
	case "globe":
		c.ring(0, 0, 0.52, col, lw)
		c.line(-0.52, 0, 0.52, 0, col, w(8))
		c.ellipseOutline(-0.26, -0.52, 0.26, 0.52, col, w(8))
	case "images":
		c.rrect(0, 0, 0.92, 0.80, 0.10, col, false, lw)
		c.polygon([][2]float64{{-0.30, 0.20}, {-0.06, -0.10}, {0.14, 0.10}, {0.30, -0.08}, {0.30, 0.20}}, bgc)
		c.disc(-0.19, -0.11, 0.07, bgc)
	case "layout":
		c.rrect(0, -0.32, 0.88, 0.30, 0.06, col, false, lw)
		c.rrect(-0.23, 0.22, 0.40, 0.34, 0.06, col, false, lw)
		c.rrect(0.23, 0.22, 0.40, 0.34, 0.06, col, false, lw)
	case "apps":
		for _, g := range [][2]float64{{-0.23, -0.23}, {0.23, -0.23}, {-0.23, 0.23}, {0.23, 0.23}} {
			c.rrect(g[0], g[1], 0.40, 0.40, 0.09, col, false, lw)
		}
	case "hourglass":
		c.rect(-0.44, -0.52, 0.44, -0.44, col)
		c.rect(-0.44, 0.44, 0.44, 0.52, col)
		c.polygon([][2]float64{{-0.38, -0.44}, {0.38, -0.44}, {0, 0}}, col)
		c.polygon([][2]float64{{-0.38, 0.44}, {0.38, 0.44}, {0, 0}}, col)
	case "megaphone":
		c.polygon([][2]float64{{-0.46, -0.30}, {0.18, -0.54}, {0.18, 0.54}, {-0.46, 0.30}}, col)
		c.rrect(0.32, 0, 0.24, 0.48, 0.06, col, false, lw)
		c.disc(-0.52, 0, 0.09, col)
	case "users":
		for _, ox := range []float64{-0.18, 0.18} {
			c.disc(ox, -0.16, 0.16, col)
			c.rrect(ox, 0.20, 0.52, 0.38, 0.14, col, false, lw)
		}
	case "heart":
		pts := [][2]float64{}
		for i := 0; i < 360; i += 8 {
			t := rad(float64(i))
			x := 16 * math.Pow(math.Sin(t), 3)
			y := 13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)
			pts = append(pts, [2]float64{x * 0.85 / 16, -y * 0.85 / 16})
		}
		c.polygon(pts, col)
	default:
		c.hexagon(0.92, col, w(16))
	}
}

// ValidateIcon 校验图标类型是否合法。
func ValidateIcon(kind string) error {
	if IsCustomIcon(kind) {
		return nil // 自定义图标按文件存在性校验，交给绘制阶段
	}
	for _, k := range IconNames() {
		if k == kind {
			return nil
		}
	}
	return errors.New("未知图标类型: " + kind)
}

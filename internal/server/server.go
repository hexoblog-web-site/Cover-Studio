// Package server 提供封面生成的 HTTP 接口。
// 设计目标：渲染快、内存低、无状态、可水平扩展。
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"cover-server/internal/render"
)

// CoverReq 与桌面版字段完全一致，便于复用前端逻辑。
type CoverReq struct {
	Left       string  `json:"left"`
	Right      string  `json:"right"`
	BG         string  `json:"bg"`
	Icon       string  `json:"icon"`
	FontColor  string  `json:"fontColor"`
	FontSize   float64 `json:"fontSize"`
	IconColor  string  `json:"iconColor"`
	IconScale  float64 `json:"iconScale"`
	Glow       float64 `json:"glow"`
	IconOnTop  bool    `json:"iconOnTop"`
	BG2        string  `json:"bg2"`
	GradDir    int     `json:"gradDir"`
	IconFrame  bool    `json:"iconFrame"`
	MonoIcon   bool    `json:"monoIcon"`
	TextShadow bool    `json:"textShadow"`
}

// Server 持有并发闸门与结果缓存。
type Server struct {
	sem   chan struct{} // 并发渲染闸门，防止突发请求打满 CPU/内存
	cache *resultCache
}

// New 创建服务。maxConcurrent<=0 时取 CPU 核数，最多 8。
func New(maxConcurrent int) *Server {
	if maxConcurrent <= 0 {
		maxConcurrent = runtime.NumCPU()
		if maxConcurrent > 8 {
			maxConcurrent = 8
		}
	}
	return &Server{
		sem:   make(chan struct{}, maxConcurrent),
		cache: newResultCache(256), // 最多缓存 256 张，内存可控
	}
}

// Register 把路由挂到 mux。
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/icons", s.handleIcons)
	mux.HandleFunc("POST /api/render", s.handleRender)
	mux.HandleFunc("POST /api/icons/upload", s.handleIconUpload)
	mux.HandleFunc("POST /api/icons/delete", s.handleIconDelete)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

// iconItem 图标选项。
type iconItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

var iconLabels = map[string]string{
	"hexagon": "六边形", "rocket": "火箭", "lock": "锁", "cpu": "芯片",
	"cloud": "云朵", "trash": "垃圾桶", "chart": "柱状图", "db": "数据库",
	"server": "服务器", "home": "主页", "book": "书本", "sparkle": "闪光",
	"star": "星星", "puzzle": "拼图", "gear": "齿轮", "chain": "链条",
	"sidebar": "侧栏", "shield": "盾牌", "tag": "标签", "clock": "时钟",
	"file": "文件", "envelope": "信封", "sliders": "滑块", "arrowcircle": "圆环箭头",
	"folder": "文件夹", "comment": "评论", "palette": "调色板", "dice": "骰子",
	"calendar": "日历", "globe": "地球", "images": "图片", "layout": "布局",
	"apps": "九宫格", "hourglass": "沙漏", "megaphone": "喇叭", "users": "用户",
	"heart": "爱心",
}

func (s *Server) handleIcons(w http.ResponseWriter, r *http.Request) {
	list := make([]iconItem, 0, len(iconLabels))
	for _, k := range render.IconNames() {
		label := iconLabels[k]
		if label == "" {
			label = k
		}
		list = append(list, iconItem{Key: k, Label: label})
	}
	// 追加用户上传的自定义图标
	ents, err := os.ReadDir(customDir)
	if err == nil {
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
				continue
			}
			label := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if len([]rune(label)) > 8 {
				label = string([]rune(label)[:8]) + "…"
			}
			list = append(list, iconItem{Key: "custom:" + e.Name(), Label: "我的:" + label})
		}
	}
	writeJSON(w, map[string]any{"ok": true, "icons": list})
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	// 限制 body 大小，防止恶意超大请求占内存
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CoverReq
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "参数解析失败: "+err.Error())
		return
	}
	if err := render.ValidateIcon(req.Icon); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	key := reqHash(req)
	// 命中缓存直接返回，零渲染开销
	if b, ok := s.cache.get(key); ok {
		writePNG(w, b, true)
		return
	}

	// 获取并发槽位（超出的请求在此排队，不占用 CPU/内存）
	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	b, err := render.Render(render.Params{
		Left: req.Left, Right: req.Right, BG: req.BG, Icon: req.Icon,
		FontColor: req.FontColor, FontSize: req.FontSize,
		IconColor: req.IconColor, IconScale: req.IconScale,
		Glow: req.Glow, IconOnTop: req.IconOnTop,
		BG2: req.BG2, GradDir: req.GradDir, IconFrame: req.IconFrame,
		MonoIcon: req.MonoIcon, TextShadow: req.TextShadow,
	})
	if err != nil {
		log.Printf("render error: %v", err)
		writeErr(w, http.StatusInternalServerError, "渲染失败: "+err.Error())
		return
	}
	s.cache.put(key, b)
	writePNG(w, b, false)
}

// reqHash 用参数 JSON 的哈希做缓存键，省略大字段不影响结果。
func reqHash(req CoverReq) string {
	b, _ := json.Marshal(req)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writePNG(w http.ResponseWriter, b []byte, cached bool) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if cached {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "msg": msg})
}

// resultCache 简单的并发安全 LRU 近似缓存：超过容量就整批清理。
// 命中率足够高、实现简单、内存上限明确。
type resultCache struct {
	mu   sync.RWMutex
	max  int
	items map[string][]byte
	order []string
}

func newResultCache(max int) *resultCache {
	return &resultCache{max: max, items: make(map[string][]byte, max)}
}

func (c *resultCache) get(k string) ([]byte, bool) {
	c.mu.RLock()
	b, ok := c.items[k]
	c.mu.RUnlock()
	return b, ok
}

func (c *resultCache) put(k string, b []byte) {
	c.mu.Lock()
	if len(c.items) >= c.max {
		// 清掉一半最旧的，避免频繁分配
		n := c.max / 2
		for i := 0; i < n && i < len(c.order); i++ {
			delete(c.items, c.order[i])
		}
		c.order = c.order[n:]
	}
	if _, exists := c.items[k]; !exists {
		c.order = append(c.order, k)
	}
	c.items[k] = b
	c.mu.Unlock()
}

package server

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cover-server/internal/render"
)

// initCustomIcons 初始化自定义图标目录（可通过环境变量 CUSTOM_ICON_DIR 挂载持久化）。
func initCustomIcons() string {
	dir := os.Getenv("CUSTOM_ICON_DIR")
	if dir == "" {
		dir = filepath.Join("data", "icons")
	}
	_ = os.MkdirAll(dir, 0o755)
	render.SetCustomIconDir(dir)
	return dir
}

var customDir string

func init() {
	customDir = initCustomIcons()
}

// handleIconUpload 接收 multipart 图片，保存到图标目录，返回可渲染的 key。
func (s *Server) handleIconUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20) // 最大 8MB
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "上传解析失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少文件字段 file: "+err.Error())
		return
	}
	defer file.Close()

	// 先解码校验是否为有效图片
	buf, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取失败: "+err.Error())
		return
	}
	if _, _, decErr := image.Decode(strings.NewReader(string(buf))); decErr != nil {
		writeErr(w, http.StatusBadRequest, "不是有效图片(PNG/JPG/GIF): "+decErr.Error())
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
		ext = ".png"
	}
	name := fmt.Sprintf("icon_%s%s", time.Now().Format("20060102_150405.000"), ext)
	if err := os.WriteFile(filepath.Join(customDir, name), buf, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "key": "custom:" + name, "name": name})
}

// handleIconDelete 删除一个自定义图标。
func (s *Server) handleIconDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	name := render.CustomIconName(body.Key)
	if !render.IsCustomIcon(body.Key) || name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		writeErr(w, http.StatusBadRequest, "非法图标名")
		return
	}
	_ = os.Remove(filepath.Join(customDir, name))
	writeJSON(w, map[string]any{"ok": true})
}

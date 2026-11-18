// 封面工坊 Web 版：一个 Go 静态二进制即可提供 Web 服务，前端已内嵌其中。
// 用法：./cover-server  （默认监听 :8080）
// 环境变量：PORT=端口  MAX_CONCURRENT=并发渲染数
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"cover-server/internal/server"
)

//go:embed all:web
var webFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	maxConc, _ := strconv.Atoi(os.Getenv("MAX_CONCURRENT"))

	srv := server.New(maxConc)
	mux := http.NewServeMux()
	srv.Register(mux)

	// 从内嵌文件系统提供静态前端
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("封面工坊已启动: http://localhost:%s", port)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

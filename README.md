# Cover Studio

一个用 Go 写的在线封面生成器：**纯色/双色渐变背景 + 居中图标 + 圆润加粗文案**，1000×500，浏览器打开即用，可部署到任何支持 Docker 的服务器。

由桌面版 `封面工坊` 移植而来，绘图核心完全复用，界面、参数与桌面版一致。

![预览](docs/preview-panda.png)

> 上图即为本程序生成：双色渐变 + 文字投影 + 自定义圆形图标 + 圆角玻璃容器。

## 特性

- 18 色背景色板 + 自定义取色，支持双色渐变与 4 个方向
- 文字颜色/字号可调，可选文字投影
- 37 个内置矢量图标，颜色/大小/光晕可调，支持文字与图标叠加顺序切换
- 支持上传自己的 PNG/JPG/GIF 图标，可选圆角玻璃容器、单色剪影
- 实时预览、一键下载 PNG
- **渲染快、内存低**：PNG 快速编码、光晕局部模糊、结果缓存、并发限流、字体全局复用、静态二进制零依赖

## 一句安装代码

在服务器上执行：

```bash
git clone https://github.com/hexoblog-web-site/Cover-Studio.git && cd Cover-Studio && docker compose up -d --build
```

完成后浏览器打开 `http://服务器IP:8080` 即可。

## 本地运行（无 Docker）

需要 Go 1.21+：

```bash
go run .
# 或编译后运行
go build -o cover-server . && ./cover-server
```

浏览器打开 <http://localhost:8080>。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8080` | 监听端口（云平台如 Railway 会自动注入） |
| `MAX_CONCURRENT` | CPU 核数（≤8） | 同时渲染的最大请求数，超出排队，保护 CPU/内存 |
| `CUSTOM_ICON_DIR` | `data/icons` | 上传图标的保存目录，Docker 中设为 `/data` 以便挂载持久化 |

## 部署到平台

- **自有服务器**：`docker compose up -d --build`，数据（上传的图标）保存在 `./data`。
- **Zeabur / Railway / Fly.io**：选择 Dockerfile 部署，设置端口 `8080`；这些平台会自己注入 `PORT`。挂载一个卷到 `/data` 可持久化用户图标（不挂载也能用，只是重启后上传的图标会清空）。
- **Vercel / Cloudflare Pages**：不支持（只能托管静态文件，跑不了 Go 服务）。

## API

### 渲染封面

```
POST /api/render
Content-Type: application/json
```

请求体（字段与桌面版一致）：

```json
{
  "left": "音乐", "right": "下载",
  "bg": "#3B82F6", "bg2": "#7C3AED", "gradDir": 0,
  "icon": "hexagon",
  "fontColor": "#FFFFFF", "fontSize": 128,
  "iconColor": "#FFFFFF", "iconScale": 1.0, "glow": 0.55,
  "iconOnTop": false, "textShadow": true,
  "iconFrame": false, "monoIcon": false
}
```

返回 `image/png`（1000×500），并带 `X-Cache: HIT/MISS` 头。相同参数会命中缓存，直接返回，零渲染开销。

其他接口：

- `GET /api/icons` — 图标列表
- `POST /api/icons/upload` — `multipart/form-data`，字段名 `file`，上传自定义图标
- `POST /api/icons/delete` — JSON `{"key":"custom:xxx.png"}`
- `GET /api/health` — 健康检查

> 直接用 API 即可批量生成：写个脚本循环 POST，把返回的 PNG 存盘，适合批量做 Hexo 封面。

## 性能说明

针对"渲染快、内存低"做的优化：

1. **PNG 快速编码**：封面是大色块渐变，用 `png.BestSpeed` 无损压缩，比默认压缩快数倍。
2. **光晕局部模糊**：原桌面版对整张 1000×500 画布做 3 次 box blur；Web 版只在图标附近的小方框内计算，计算量降一个量级，视觉一致。
3. **结果缓存**：相同参数直接返回缓存 PNG，用 LRU 近似策略，最多 256 张，内存上限明确。
4. **并发限流**：用带缓冲 channel 做闸门，超出 `MAX_CONCURRENT` 的请求排队，避免突发流量打满 CPU/内存。
5. **字体全局复用**：TTF 字体用 `sync.Once` 解析一次（约 10MB），全程共享，不重复加载。
6. **单静态二进制**：前端用 `go:embed` 内嵌，最终镜像基于 `scratch`，只有十几 MB，启动毫秒级。

实测：单张渲染约几十毫秒，镜像约 20MB（含证书）。

## 目录结构

```
Cover-Studio/
├── main.go                 # 入口：内嵌前端 + HTTP 服务
├── internal/
│   ├── render/             # 绘图核心（从桌面版复用）
│   │   ├── render.go
│   │   ├── custom.go
│   │   └── STXIHEI.TTF     # 内嵌圆润字体
│   └── server/             # HTTP 接口、缓存、上传
├── web/                    # 前端页面（内嵌进二进制）
├── data/icons/             # 上传的自定义图标（运行时生成，可挂载持久化）
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 字体说明

内嵌字体为 `internal/render/STXIHEI.TTF`，通过 `go:embed` 打进二进制，保证任何环境下渲染效果一致。

该字体版权归原厂商所有，仅随本项目用于学习与个人使用；**如需商用请替换为可商用字体**（如思源黑体、阿里巴巴普惠体），替换后重新 `go build` 即可，代码中对字号/加粗的处理会自动适配。

## 许可

代码部分 MIT（可自行修改后商用/自用）。

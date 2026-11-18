# ---------- 构建阶段 ----------
FROM golang:1.27-alpine AS build
WORKDIR /src

# 先复制依赖文件，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 纯静态二进制：不依赖 libc，可放进 scratch
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /cover-server .

# ---------- 运行阶段 ----------
FROM scratch
# 时区/证书（可选，让 HTTPS 拉取更稳）
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /cover-server /cover-server

ENV PORT=8080
ENV CUSTOM_ICON_DIR=/data/icons
VOLUME ["/data"]

EXPOSE 8080
ENTRYPOINT ["/cover-server"]

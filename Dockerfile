# ============================================================
# 阶段 1：构建前端
# ============================================================
FROM node:18-alpine AS frontend-builder

WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ============================================================
# 阶段 2：构建后端
# ============================================================
FROM golang:1.24-alpine AS backend-builder

# CGO 需要 gcc（SQLite 驱动依赖）
RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY tools.go ./
RUN CGO_ENABLED=1 go build -o auto-deploy-platform ./cmd/

# ============================================================
# 阶段 3：最终运行镜像
# ============================================================
FROM alpine:3.20

# 安装运行时依赖：git（拉取代码）、openssh-client（SSH 部署）、ca-certificates（HTTPS）
RUN apk add --no-cache git openssh-client ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从构建阶段复制产物
COPY --from=backend-builder /build/auto-deploy-platform ./
COPY --from=frontend-builder /build/web/dist ./web/dist/
COPY config.example.yaml ./config.example.yaml

# 创建数据和工作目录
RUN mkdir -p /app/data /app/workspace

# 默认环境变量
ENV GIN_MODE=release
ENV CONFIG_PATH=/app/config.yaml
ENV DB_PATH=/app/data/deploy.db

# 暴露端口
EXPOSE 8080

# 数据卷：配置文件和数据库持久化
VOLUME ["/app/data"]

ENTRYPOINT ["./auto-deploy-platform"]

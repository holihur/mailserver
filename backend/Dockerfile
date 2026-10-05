# 单镜像同时提供 API + 内嵌前端。
# 构建上下文为仓库根目录：docker build -f backend/Dockerfile .
#
# --- 1) 构建前端 ---
FROM node:26-alpine AS web
WORKDIR /web
RUN npm i -g pnpm@11.25.0
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ .
RUN pnpm run build

# --- 2) 构建后端（把前端嵌入二进制）---
FROM golang:1.27-alpine AS build
# 纯 Go 构建（PostgreSQL 驱动），无需 CGO/gcc
ENV CGO_ENABLED=0 GOFLAGS=-p=1
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=web /web/dist ./web
# 纯 Go（无 CGO），静态二进制可直接跑在 distroless/static
RUN go build -ldflags="-s -w" -o mailserver .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /app/mailserver .
VOLUME /data
ENV PORT=8080 DATA_DIR=/data CERT_DIR=/data/certs
EXPOSE 8080 5353/udp 5353/tcp 2525 25 587 465 110 143 995 993
ENTRYPOINT ["./mailserver"]

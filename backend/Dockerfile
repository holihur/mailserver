# 单镜像同时提供 API + 内嵌前端。
# 构建上下文为仓库根目录：docker build -f backend/Dockerfile .
#
# --- 1) 构建前端 ---
FROM node:22-alpine AS web
WORKDIR /web
RUN npm i -g pnpm@11.25.0
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ .
RUN pnpm run build

# --- 2) 构建后端（把前端嵌入二进制）---
FROM golang:1.22-alpine AS build
# sqlite 驱动(mattn)必须 CGO 编译；GOFLAGS=-p=1 串行编译省内存
RUN apk add --no-cache gcc musl-dev
ENV CGO_ENABLED=1 GOFLAGS=-p=1
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=web /web/dist ./web
# musl + CGO(sqlite) 静态链接，才能在 distroless/static 运行
RUN go build -ldflags="-s -w -extldflags '-static'" -o mailserver .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /app/mailserver .
VOLUME /data
ENV PORT=8080 DB_PATH=/data/mail.db CERT_DIR=/data/certs
EXPOSE 8080 2525 25 587 465 110 143 995 993
ENTRYPOINT ["./mailserver"]

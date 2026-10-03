# 低内存：golang:1.22-alpine 构建，scratch 运行 (~12MB)
FROM golang:1.22-alpine AS build
# sqlite 驱动(mattn)必须 CGO 编译；GOFLAGS=-p=1 串行编译省内存(小内存机器)
RUN apk add --no-cache gcc musl-dev
ENV CGO_ENABLED=1 GOFLAGS=-p=1
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o mailserver .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /app/mailserver .
VOLUME /data
ENV PORT=8080 DB_PATH=/data/mail.db
EXPOSE 8080 2525 587 110 143
ENTRYPOINT ["./mailserver"]

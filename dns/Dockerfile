# 自研权威 DNS，scratch 运行（~10MB，常驻 8~15MB）
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o nsd .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /app/nsd .
VOLUME /data
ENV DNS_ADDR=:53 ZONES_PATH=/data/zones.json
EXPOSE 53/udp 53/tcp
ENTRYPOINT ["./nsd"]

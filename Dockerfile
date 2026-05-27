# 多阶段构建 - 构建阶段
FROM golang:1.25-alpine AS builder

WORKDIR /build

# 复制依赖文件并下载
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码并构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o codesome .

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 复制构建产物
COPY --from=builder /build/codesome .

EXPOSE 8080

ENTRYPOINT ["./codesome"]
CMD ["serve"]

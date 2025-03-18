FROM golang:1.24-alpine AS builder

WORKDIR /app

# 复制Go模块定义
COPY go.mod go.sum* ./
RUN go mod download

# 复制源代码
COPY . .

# 编译程序
RUN go build -o update-apt-source .

# 使用较小的镜像作为最终容器
FROM alpine:latest

RUN apk --no-cache add ca-certificates gpg gpg-agent

WORKDIR /app

# 复制编译好的二进制文件
COPY --from=builder /app/update-apt-source /app/update-apt-source

# 确保二进制文件可执行
RUN chmod +x /app/update-apt-source

# 设置入口点
ENTRYPOINT ["/app/update-apt-source"]

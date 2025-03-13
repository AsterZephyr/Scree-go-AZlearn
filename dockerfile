# 第一阶段：构建应用
FROM golang:1.23-alpine AS builder

# 安装必要的构建工具
RUN apk add --no-cache git ca-certificates

# 设置工作目录
WORKDIR /app

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o scree-go-azlearn main.go

# 第二阶段：创建最终镜像
FROM scratch

# 从构建阶段复制SSL证书
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 从构建阶段复制可执行文件
COPY --from=builder /app/scree-go-azlearn /scree-go-azlearn

# 设置用户
USER 1001

# 暴露HTTP和TURN服务器端口
EXPOSE 3478/tcp
EXPOSE 3478/udp
EXPOSE 5050

# 设置工作目录
WORKDIR "/"

# 设置容器启动命令
ENTRYPOINT ["/scree-go-azlearn"]
CMD ["serve"]

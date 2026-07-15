#!/bin/bash
set -e

echo "=== NetBerth 构建 & 部署 ==="
echo ""

# Step 1: Build on local machine
echo "[1/4] 构建 Docker 镜像..."
docker build -t netberth:latest .

# Step 2: Export image
echo "[2/4] 导出镜像..."
docker save netberth:latest | gzip > netberth.tar.gz
echo "镜像已导出: netberth.tar.gz ($(du -h netberth.tar.gz | cut -f1))"

# Step 3: Ask for target
echo ""
echo "[3/4] 传输到服务器"
echo "选择目标服务器:"
echo "  1) Unraid (通过 SCP)"
echo "  2) 群晖 NAS (通过 SCP)"
echo "  3) 跳过传输，我手动处理"
read -p "选择 [1-3]: " choice

case $choice in
  1|2)
    read -p "服务器 IP: " server_ip
    read -p "SSH 用户名: " server_user
    read -p "目标路径 (如 /mnt/user/appdata/): " target_path

    echo "上传项目文件..."
    # Create a tar of the project (excluding git and node_modules)
    cd "$(dirname "$0")/.."
    tar --exclude='.git' --exclude='node_modules' --exclude='web/node_modules' \
        -czf /tmp/netberth-project.tar.gz .

    scp netberth.tar.gz /tmp/netberth-project.tar.gz \
        "${server_user}@${server_ip}:${target_path}/"

    echo "文件已上传到 ${server_ip}:${target_path}/"
    echo ""
    echo "=== 在服务器上执行以下命令 ==="
    echo "cd ${target_path}"
    echo "tar -xzf netberth-project.tar.gz"
    echo "gunzip -c netberth.tar.gz | docker load"
    echo "docker compose -f docker-compose.prod.yml up -d"
    echo "docker compose logs | grep 'ADMIN CREDENTIALS'"
    ;;
  3)
    echo ""
    echo "=== 手动部署 ==="
    echo "1. 将 netberth.tar.gz 和项目文件传到服务器"
    echo "2. 在服务器上执行:"
    echo "   gunzip -c netberth.tar.gz | docker load"
    echo "   docker compose -f docker-compose.prod.yml up -d"
    echo "   docker compose logs | grep 'ADMIN CREDENTIALS'"
    ;;
esac

echo ""
echo "[4/4] 完成!"
echo "管理面板: http://<服务器IP>:8443"
echo "默认用户: admin"
echo "密码: 查看服务器日志"

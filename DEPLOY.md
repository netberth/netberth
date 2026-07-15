# NetBerth 部署指南

## 方案一：Unraid 服务器

Unraid 原生支持 Docker，6.12+ 版本支持 docker compose。

### 步骤

```bash
# 1. SSH 登录 Unraid
ssh root@your-unraid-ip

# 2. 找一个合适的位置（比如 /mnt/user/appdata/）
cd /mnt/user/appdata/

# 3. 克隆项目（或手动上传文件夹）
git clone <your-repo-url> netberth
cd netberth

# 4. 构建并启动
docker compose -f docker-compose.prod.yml up -d --build

# 5. 查看首次管理员密码
docker compose logs netberth | grep "ADMIN CREDENTIALS"

# 6. 访问管理面板
# http://your-unraid-ip:8443
```

### 构建时间
- 首次构建约 3-5 分钟（下载 Go 依赖 + 编译）
- 后续构建利用 Docker 缓存，约 30 秒

### 重要：代理端口
反向代理功能默认监听 80 端口（host 网络模式）。如果 Unraid WebUI 也占用了 80，可以在配置中修改：
```yaml
environment:
  - NB_PROXY_PORT=8080  # 改为其他端口
```

## 方案二：群晖 NAS

群晖 DSM 7.x 使用 Container Manager（原 Docker）。

### 步骤 A：通过 SSH + docker compose

```bash
# 1. SSH 登录（需先在 DSM 控制面板 > 终端机 中启用 SSH）
ssh admin@your-synology-ip

# 2. 进入 docker 共享文件夹
cd /volume1/docker/

# 3. 克隆项目
git clone <your-repo-url> netberth
cd netberth

# 4. 构建并启动
sudo docker compose -f docker-compose.prod.yml up -d --build

# 5. 查看管理员密码
sudo docker compose logs netberth | grep "ADMIN CREDENTIALS"

# 6. 访问
# http://your-synology-ip:8443
```

### 步骤 B：通过 Container Manager 界面

1. 打开 Container Manager
2. 项目 → 新增 → 设置项目名称 `netberth`
3. 路径选择 `/volume1/docker/netberth`
4. 来源选择 "设置路径" 或直接粘贴 `docker-compose.prod.yml` 内容
5. 下一步 → 完成 → 项目会自动构建和启动
6. 在容器列表中点击 netberth → 详情 → 日志，查看初始管理员密码

## 部署后检查

```bash
# 健康检查
curl http://localhost:8443/api/v1/system/status

# 预期返回：
# {"success":true,"data":{"hostname":"...","version":"0.1.0",...}}

# 查看实时日志
docker compose logs -f netberth
```

## 首次登录

1. 浏览器打开 `http://<设备IP>:8443`
2. 用户名：`admin`
3. 密码：从日志中获取（搜索 `ADMIN CREDENTIALS`）
4. 登录后立即在 Settings 页面修改密码

## 持久化目录

| 目录 | 用途 |
|------|------|
| `./data/` | SQLite 数据库 + JWT 密钥 |
| `./certs/` | SSL 证书存储 |

删除这些目录会丢失所有配置（重置为初始状态）。

## 故障排查

### 端口冲突
如果 `docker compose logs` 显示 `bind: address already in use`：
```bash
# 查看哪些端口被占用
netstat -tlnp | grep -E ":(8443|80) "

# 修改环境变量更换端口
NB_SERVER_PORT=9443 docker compose -f docker-compose.prod.yml up -d
```

### 构建失败
如果 Go 依赖下载超时（国内网络）：
```bash
# 在 Dockerfile 的 go mod download 之前设置代理
# 或在 docker-compose.prod.yml 中添加：
environment:
  - GOPROXY=https://goproxy.cn,direct
```

### 查看日志
```bash
docker compose logs -f --tail=50 netberth
```

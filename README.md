# NginxFlow

基于 Web 的 Nginx 反向代理管理系统，支持 HTTP/HTTPS/TCP/UDP 多协议转发、SSL 证书管理、健康检查、实时日志、主从节点同步。

---

## 功能概览

### 转发规则
- 支持 **HTTP / HTTPS / TCP / UDP** 四种协议
- HTTP 规则支持同时监听 HTTP 和 HTTPS 端口，可开启 HTTP → HTTPS 自动 301 跳转
- 支持多域名绑定（空格分隔），未匹配域名自动返回「网站不存在」页面
- IPv4 / IPv6 / 双栈监听模式可选
- 负载均衡算法：**轮询 / IP 哈希 / 最少连接**
- 后端节点权重配置，支持单独禁用节点
- 自定义 nginx 指令（高级模式）

### 健康检查
- 支持 HTTP / TCP / UDP 三种探针
- 可配置检查间隔、超时、连续失败下线次数、连续成功恢复次数
- 规则列表实时展示节点在线状态（绿 / 黄 / 红）

### SSL 证书
- 上传证书文件自动校验证书与私钥是否匹配
- 自动提取域名（SAN / CN）
- 支持腾讯云 DNS API 自动续签（配置 SecretId / SecretKey）

### 实时日志
- 规则列表内直接打开终端风格日志抽屉
- 基于 SSE 实时推送，支持暂停 / 继续 / 清空 / 滚动到最新
- HTTP 规则查看 access log，TCP/UDP 查看 stream log

### 主从节点同步
- 主节点推送配置到多个从节点
- Token 鉴权，从节点仅接收不可主动写入

### 系统设置
- nginx 全局参数（worker 进程数、最大连接数、keepalive 超时、最大请求体等）
- 日志自动轮转大小配置
- 腾讯云 SSL 续签凭据
- Mattermost Webhook 告警通知
- 一键导出系统备份（JSON）
- 一键测试 nginx 配置语法 / 重载 nginx

### 安全
- JWT 鉴权，Token 24 小时有效
- 30 分钟无操作自动退出登录
- 登录页不预填默认账号密码

---

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go · Gin · SQLite（WAL 模式） |
| 前端 | Vue 3 · Vite · Element Plus |
| 代理 | Nginx（需 stream 模块） |

---

## 部署要求

- Ubuntu 20.04 / 22.04（推荐）或其他 Linux 发行版
- Nginx（含 `ngx_stream_module`）
- Go 1.21+（编译时需要）
- Node.js 18+（编译前端时需要）

---

## 一键安装

> 支持 Ubuntu 20.04 / 22.04 / 24.04，缺少的依赖（Go、Node.js、Nginx）会自动安装。

```bash
curl -fsSL https://raw.githubusercontent.com/AnkerYe99/nginxflow/master/install.sh | sudo bash
```

或先下载再执行：

```bash
wget https://raw.githubusercontent.com/AnkerYe99/nginxflow/master/install.sh
chmod +x install.sh
sudo ./install.sh
```

安装过程会询问管理后台端口（默认 `9001`）和 API 端口（默认 `9000`），直接回车使用默认值即可。

安装完成后访问 `http://<服务器IP>:9001`，默认账号 `admin` / `admin123`，**首次登录后请修改密码**。

---

## 手动部署

### 1. 克隆仓库

```bash
git clone https://github.com/AnkerYe99/nginxflow.git
cd nginxflow
```

### 2. 编译后端

```bash
cd backend
go build -o nginxflow-server .
```

### 3. 编译前端

```bash
cd frontend
npm install
npm run build
# 产物在 frontend/dist/
```

### 4. 配置文件

创建 `/opt/nginxflow/config.yaml`：

```yaml
server:
  port: 9000

nginx:
  conf_dir: /etc/nginx/conf.d
  reload_cmd: nginx -s reload
  test_cmd: nginx -t

db:
  path: /opt/nginxflow/data/nginxflow.db

jwt:
  secret: 请替换为随机字符串
```

### 5. Nginx 主配置

参考 `deploy/nginx.conf`，将前端静态文件目录指向 `/opt/nginxflow/frontend`，API 反代到 `127.0.0.1:9000`。

### 6. 注册系统服务

```bash
sudo cp deploy/nginxflow.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nginxflow
```

### 7. 初始账号

```
用户名：admin
密码：admin123
```

首次登录后请在系统设置中修改密码。

---

## 目录结构

```
nginxflow/
├── backend/
│   ├── config/      # 配置加载
│   ├── db/          # 数据库初始化与迁移
│   ├── engine/      # nginx 配置生成、日志轮转
│   ├── handler/     # HTTP 接口处理
│   ├── health/      # 健康检查探针
│   ├── middleware/  # JWT 鉴权中间件
│   ├── model/       # 数据模型
│   ├── util/        # 通用工具
│   └── main.go
├── frontend/
│   ├── src/
│   │   ├── views/   # 页面组件
│   │   ├── api.js   # Axios 封装
│   │   └── router/  # 路由
│   └── package.json
└── deploy/
    ├── nginx.conf        # 参考 nginx 配置
    └── nginxflow.service # systemd 服务文件
```

---

## License

Private — All rights reserved © [AnkerYe](mailto:AnkerYe@gmail.com)

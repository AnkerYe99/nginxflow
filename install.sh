#!/bin/bash
set -e

# ============================================================
#  AnkerYe - 流量管理 (AnkerYe-BTM) 一键安装脚本
#
#  GitHub（公网）:
#    curl -sSL https://raw.githubusercontent.com/AnkerYe99/AnkerYe-BTM/master/install.sh | bash
#
#  内网 Gitea:
#    GITEA_URL=http://10.14.6.51:3000 bash <(curl -sSL http://10.14.6.51:3000/anker/AnkerYe-BTM/raw/branch/master/install.sh)
#
#  自定义端口:
#    BTM_PORT=8080 bash install.sh
# ============================================================

GITEA_REPO="anker/AnkerYe-BTM"
GITHUB_REPO="AnkerYe99/AnkerYe-BTM"
INSTALL_DIR="/opt/AnkerYe-BTM"
DATA_DIR="$INSTALL_DIR/data"
LOG_DIR="/var/log/AnkerYe-BTM"
SERVICE_NAME="AnkerYe-BTM"
PORT="${BTM_PORT:-9000}"

# 自动识别架构
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) BIN_NAME="ankerye-flow-server-arm64" ;;
  x86_64)        BIN_NAME="ankerye-flow-server-amd64" ;;
  *)             error "不支持的架构: $ARCH（仅支持 x86_64 / aarch64）" ;;
esac

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()  { echo -e "${GREEN}[✓]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }
step()  { echo -e "\n${BLUE}▶ $*${NC}"; }

echo -e "${BLUE}"
cat << 'BANNER'
    _         _              __   __
   / \   _ __ | | _____ _ __ \ \ / ___
  / _ \ | '_ \| |/ / _ \ '__|  \ V / _ \
 / ___ \| | | |   <  __/ |      | |  __/
/_/   \_\_| |_|_|\_\___|_|      |_|\___|
BANNER
echo -e "${NC}"
echo "  AnkerYe-BTM 反向代理流量管理系统  |  安装端口: $PORT"
echo "  ─────────────────────────────────────────────────────"

# ── 权限检查 ────────────────────────────────────────────────
[[ $EUID -ne 0 ]] && error "请以 root 或 sudo 运行此脚本"

# ── 系统检查 ────────────────────────────────────────────────
step "检查系统环境"
command -v apt-get &>/dev/null || error "目前仅支持 Debian/Ubuntu 系统"
export DEBIAN_FRONTEND=noninteractive
for pkg in curl python3; do
  if ! command -v $pkg &>/dev/null; then
    info "安装 $pkg..."
    apt-get update -y && apt-get install -y $pkg
  fi
done
# 确保 cron 守护进程已安装并运行（日志轮转依赖）
if ! dpkg -l cron 2>/dev/null | grep -q '^ii'; then
  info "安装 cron..."
  apt-get update -y && apt-get install -y --no-install-recommends cron
fi
systemctl enable cron 2>/dev/null && systemctl start cron 2>/dev/null || true
info "系统: $(lsb_release -ds 2>/dev/null || uname -sr)"

# ── 探测下载源 ────────────────────────────────────────────
step "探测下载源"
DL_TYPE=""
DL_BASE=""
# 默认内网 Gitea 地址，可通过环境变量覆盖
GITEA_URL="${GITEA_URL:-http://10.14.6.51:3000}"

_get_release_json() {
  if [[ "$DL_TYPE" == "gitea" ]]; then
    curl -sf --connect-timeout 8 "$DL_BASE/api/v1/repos/$GITEA_REPO/releases/latest"
  else
    curl -sf --connect-timeout 8 "https://api.github.com/repos/$GITHUB_REPO/releases/latest"
  fi
}

_parse_tag() {
  python3 -c "import sys,json; print(json.load(sys.stdin)['tag_name'])" 2>/dev/null
}

_parse_asset_url() {
  python3 -c "
import sys, json
assets = json.load(sys.stdin).get('assets', [])
url = next((a['browser_download_url'] for a in assets if a['name'] == '$BIN_NAME'), '')
print(url)
" 2>/dev/null
}

# 优先内网 Gitea
if curl -sf --connect-timeout 5 "$GITEA_URL/api/v1/repos/$GITEA_REPO" -o /dev/null 2>/dev/null; then
  DL_TYPE="gitea"
  DL_BASE="${GITEA_URL%/}"
  info "内网 Gitea: $DL_BASE"
else
  warn "Gitea ($GITEA_URL) 不可达，尝试公网 GitHub..."
fi

# 回退 GitHub
if [[ -z "$DL_TYPE" ]]; then
  if curl -sf --connect-timeout 8 "https://api.github.com/repos/$GITHUB_REPO/releases/latest" -o /dev/null 2>/dev/null; then
    DL_TYPE="github"
    info "公网 GitHub: github.com/$GITHUB_REPO"
  fi
fi

[[ -z "$DL_TYPE" ]] && error "无法连接任何下载源（Gitea: $GITEA_URL，GitHub 均不可达）"

# ── 下载二进制 ─────────────────────────────────────────────
step "下载最新版本"
RELEASE_JSON=$(_get_release_json) || error "无法获取 Release 信息"
LATEST_TAG=$(echo "$RELEASE_JSON" | _parse_tag)
DL_URL=$(echo "$RELEASE_JSON" | _parse_asset_url)

[[ -z "$LATEST_TAG" ]] && error "无法解析版本号，请确认已在仓库中创建 Release"
[[ -z "$DL_URL" ]]     && error "Release 中未找到 $BIN_NAME 二进制，请先上传构建产物"

info "最新版本: $LATEST_TAG"
info "下载地址: $DL_URL"

DL_TMP=$(mktemp /tmp/ankerye-btm-XXXXXX)
curl -fL --progress-bar -o "$DL_TMP" "$DL_URL" || error "下载失败"
chmod +x "$DL_TMP"
info "下载完成"

# ── 安装 Nginx ──────────────────────────────────────────────
step "检查 Nginx"
if command -v nginx &>/dev/null; then
  info "Nginx 已安装: $(nginx -v 2>&1)"
else
  info "Nginx 未安装，正在安装（可能需要1-2分钟）..."
  apt-get update -y
  apt-get install -y --no-install-recommends nginx
  info "Nginx 安装完成: $(nginx -v 2>&1)"
fi

# ── 停止旧服务 ──────────────────────────────────────────────
step "准备安装目录"
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  warn "停止现有服务..."
  systemctl stop "$SERVICE_NAME"
fi
mkdir -p "$DATA_DIR" "$LOG_DIR"
cp "$DL_TMP" "$INSTALL_DIR/ankerye-flow-server"
rm -f "$DL_TMP"
chmod +x "$INSTALL_DIR/ankerye-flow-server"
info "二进制已安装到 $INSTALL_DIR/ankerye-flow-server"

# ── 下载 IP 地理库 ────────────────────────────────────────────
step "下载 IP 地理库"
MMDB_PATH="$DATA_DIR/GeoLite2-City.mmdb"
GITEA_MMDB="http://10.14.6.51:3000/anker/AnkerYe-BTM/raw/branch/main/data/GeoLite2-City.mmdb.gz"
if [[ -f "$MMDB_PATH" ]]; then
  warn "IP 地理库已存在，跳过下载（如需更新请手动删除 $MMDB_PATH 后重装）"
else
  MMDB_TMP="$(mktemp /tmp/dbip-XXXXXX.mmdb.gz)"
  # 优先从本地 Gitea 仓库下载（内网，秒级完成）
  info "从本地 Gitea 下载 IP 地理库..."
  if curl -fL --connect-timeout 10 --max-time 120 --progress-bar "$GITEA_MMDB" -o "$MMDB_TMP" 2>/dev/null && gunzip -c "$MMDB_TMP" > "$MMDB_PATH"; then
    rm -f "$MMDB_TMP"
    info "IP 地理库已下载（本地 Gitea）: $MMDB_PATH ($(du -sh "$MMDB_PATH" | cut -f1))"
  else
    # Gitea 不可达则回退公网
    warn "本地 Gitea 不可达，回退公网下载..."
    YYYYMM=$(date +%Y-%m)
    MMDB_URL="https://download.db-ip.com/free/dbip-city-lite-${YYYYMM}.mmdb.gz"
    if curl -fL --connect-timeout 30 --max-time 180 --progress-bar "$MMDB_URL" -o "$MMDB_TMP" 2>/dev/null && gunzip -c "$MMDB_TMP" > "$MMDB_PATH"; then
      rm -f "$MMDB_TMP"
      info "IP 地理库已下载（公网 $YYYYMM）: $MMDB_PATH ($(du -sh "$MMDB_PATH" | cut -f1))"
    else
      # 尝试上个月版本
      PREV_MM=$(date -d "$(date +%Y-%m-01) -1 month" +%Y-%m 2>/dev/null || date -v-1m +%Y-%m 2>/dev/null || echo "")
      MMDB_URL2="https://download.db-ip.com/free/dbip-city-lite-${PREV_MM}.mmdb.gz"
      if [[ -n "$PREV_MM" ]] && curl -fL --connect-timeout 30 --max-time 180 --progress-bar "$MMDB_URL2" -o "$MMDB_TMP" 2>/dev/null && gunzip -c "$MMDB_TMP" > "$MMDB_PATH"; then
        rm -f "$MMDB_TMP"
        info "IP 地理库已下载（公网上月 $PREV_MM）: $MMDB_PATH"
      else
        rm -f "$MMDB_TMP" "$MMDB_PATH"
        warn "IP 地理库下载失败，GeoIP 功能暂不可用，后续可手动下载到 $MMDB_PATH"
      fi
    fi
  fi
fi

# ── 生成配置 ────────────────────────────────────────────────
if [[ ! -f "$INSTALL_DIR/config.yaml" ]]; then
  step "生成配置文件"
  JWT_SECRET=$(openssl rand -base64 48 | tr -dc 'a-zA-Z0-9!@#^&*' | head -c 48)
  cat > "$INSTALL_DIR/config.yaml" << CFEOF
server:
  port: $PORT
  jwt_secret: "$JWT_SECRET"
  jwt_expire_hours: 24

database:
  path: $DATA_DIR/ankerye-btm.db

nginx:
  conf_dir: /etc/nginx/conf.d
  cert_dir: /etc/nginx/certs
  log_dir: $LOG_DIR
  logrotate_dir: /etc/logrotate.d
  reload_cmd: nginx -s reload
  test_cmd: nginx -t

health_check:
  default_interval: 10
  default_timeout: 3

cert:
  renew_before_days: 10
  check_hour: 2

geoip_db: $DATA_DIR/GeoLite2-City.mmdb
CFEOF
  info "配置文件已生成"
else
  warn "config.yaml 已存在，保留原有配置（升级模式）"
fi

# ── 配置 Nginx ──────────────────────────────────────────────
step "配置 Nginx"
[[ ! -f /etc/nginx/nginx.conf.btm-bak ]] && cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.btm-bak

# 检测是否有 stream 模块
STREAM_MOD=""
for f in /usr/lib/nginx/modules/ngx_stream_module.so /usr/share/nginx/modules/ngx_stream_module.so; do
  [[ -f "$f" ]] && STREAM_MOD="load_module $f;" && break
done

cat > /etc/nginx/nginx.conf << NGINXEOF
${STREAM_MOD}

user www-data;
worker_processes auto;
pid /run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include       /etc/nginx/mime.types;
    default_type  application/octet-stream;
    sendfile      on;
    keepalive_timeout 65;
    client_max_body_size 64m;
    log_format ankerye_flow '\$remote_addr - \$remote_user [\$time_local] "\$request" '
                            '\$status \$body_bytes_sent "\$http_referer" "\$http_user_agent" \$upstream_addr '
                            '\$request_time \$upstream_response_time';
    log_format ankerye_capture escape=json '{"time":"\$time_iso8601","ip":"\$remote_addr",'
                            '"method":"\$request_method","uri":"\$request_uri","status":\$status,'
                            '"req_time":\$request_time,"up_time":"\$upstream_response_time",'
                            '"upstream":"\$upstream_addr","content_type":"\$content_type",'
                            '"ua":"\$http_user_agent","body":"\$request_body"}';
    include /etc/nginx/conf.d/*-http.conf;
}
NGINXEOF

if [[ -n "$STREAM_MOD" ]]; then
  cat >> /etc/nginx/nginx.conf << 'NGINXEOF'

stream {
    log_format basic '$remote_addr [$time_local] $protocol $status '
                     '$bytes_sent $bytes_received $session_time "$upstream_addr"';
    include /etc/nginx/conf.d/*-stream.conf;
}
NGINXEOF
fi

mkdir -p /etc/nginx/certs
nginx -t 2>/dev/null && {
  if pgrep -x nginx &>/dev/null; then nginx -s reload; else nginx; fi
}
info "Nginx 配置完成"

# ── 配置日志轮转 ─────────────────────────────────────────────
step "配置日志轮转"

# app.log 轮转（copytruncate，无需重启服务）
cat > /etc/logrotate.d/ankerye-flow-applog << 'LREOF'
/var/log/AnkerYe-BTM/app.log {
    size 10M
    rotate 3
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
LREOF
info "app.log logrotate 配置完成"

# 每 10 分钟运行 logrotate（确保单个大文件能及时处理）
cat > /etc/cron.d/btm-logrotate << 'CREOF'
*/10 * * * * root /usr/sbin/logrotate /etc/logrotate.d/ankerye-flow-* 2>/dev/null
CREOF
chmod 644 /etc/cron.d/btm-logrotate
info "logrotate cron 已配置（每 10 分钟执行）"

# 注册系统 logrotate 配置（cron.daily 补充）
if [[ -f /etc/logrotate.d/nginx ]]; then
  info "系统 nginx logrotate 已存在，跳过"
fi
systemctl is-active --quiet cron 2>/dev/null || { systemctl enable cron && systemctl start cron; }
info "cron 服务已运行"

# ── 注册 systemd 服务 ────────────────────────────────────────
step "注册系统服务"
cat > /etc/systemd/system/$SERVICE_NAME.service << SVCEOF
[Unit]
Description=AnkerYe - 流量管理
After=network.target nginx.service

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/ankerye-flow-server -config $INSTALL_DIR/config.yaml
Restart=on-failure
RestartSec=5
Environment=TZ=Asia/Shanghai
StandardOutput=append:$LOG_DIR/app.log
StandardError=append:$LOG_DIR/app.log

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
sleep 2

if systemctl is-active --quiet "$SERVICE_NAME"; then
  info "服务启动成功"
else
  error "服务启动失败，请查看: journalctl -u $SERVICE_NAME -n 30"
fi

# ── 修复已知问题 ─────────────────────────────────────────────
step "修复存量配置"

# 1. 旧版 nginx.conf 日志格式名 ankerye_btm → ankerye_flow
if grep -q "ankerye_btm" /etc/nginx/nginx.conf 2>/dev/null; then
  sed -i 's/ankerye_btm/ankerye_flow/g' /etc/nginx/nginx.conf
  info "nginx.conf 日志格式名已修正为 ankerye_flow"
fi

# 2. 确保 nginx 日志目录存在
mkdir -p "$LOG_DIR"
if ls /etc/nginx/conf.d/*-http.conf &>/dev/null 2>&1; then
  for f in /etc/nginx/conf.d/*-http.conf; do
    dir=$(grep -oP 'access_log \K[^ ]+' "$f" 2>/dev/null | xargs dirname 2>/dev/null || true)
    [[ -n "$dir" ]] && mkdir -p "$dir"
  done
fi

# 3. 从 SQLite 将证书写入磁盘（避免 nginx 因缺文件 reload 失败）
DB_PATH="$DATA_DIR/ankerye-btm.db"
CERT_DIR="/etc/nginx/certs"
if [[ -f "$DB_PATH" ]]; then
  python3 - "$DB_PATH" "$CERT_DIR" <<'PYEOF'
import sys, sqlite3, os
db_path, cert_dir = sys.argv[1], sys.argv[2]
try:
    c = sqlite3.connect(db_path)
    rows = c.execute("SELECT domain, cert_pem, key_pem FROM ssl_certs").fetchall()
    written = 0
    for domain, cert, key in rows:
        if not (domain and cert and key):
            continue
        d = os.path.join(cert_dir, domain)
        os.makedirs(d, exist_ok=True)
        open(os.path.join(d, "fullchain.pem"), "w").write(cert)
        kf = os.path.join(d, "privkey.pem")
        open(kf, "w").write(key)
        os.chmod(kf, 0o600)
        written += 1
    print(f"[证书] 写入 {written} 个域名证书到磁盘")
except Exception as e:
    print(f"[证书] 跳过（{e}）")
PYEOF
fi

# 4. nginx 最终测试并 reload
if nginx -t 2>/dev/null; then
  if pgrep -x nginx &>/dev/null; then
    nginx -s reload && info "Nginx reload 成功"
  else
    nginx && info "Nginx 启动成功"
  fi
else
  warn "nginx -t 仍有错误，请手动检查: nginx -t"
fi

# ── 完成 ────────────────────────────────────────────────────
IP=$(hostname -I | awk '{print $1}')
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║        AnkerYe-BTM 安装完成！                        ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════╣${NC}"
printf "${GREEN}║${NC}  版本:     ${BLUE}%-38s${GREEN}║${NC}\n" "$LATEST_TAG"
printf "${GREEN}║${NC}  访问地址: ${BLUE}%-38s${GREEN}║${NC}\n" "http://$IP:$PORT"
printf "${GREEN}║${NC}  默认账号: ${YELLOW}%-38s${GREEN}║${NC}\n" "admin"
printf "${GREEN}║${NC}  默认密码: ${YELLOW}%-38s${GREEN}║${NC}\n" "admin123"
printf "${GREEN}║${NC}  配置文件: %-38s${GREEN}║${NC}\n" "$INSTALL_DIR/config.yaml"
printf "${GREEN}║${NC}  数据目录: %-38s${GREEN}║${NC}\n" "$DATA_DIR"
printf "${GREEN}║${NC}  运行日志: %-38s${GREEN}║${NC}\n" "$LOG_DIR/app.log"
echo -e "${GREEN}║${NC}  服务管理: systemctl [start|stop|restart|status] $SERVICE_NAME"
echo -e "${GREEN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""

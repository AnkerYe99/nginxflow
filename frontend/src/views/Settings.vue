<template>
  <div>
    <h2>系统设置</h2>
    <el-card>
      <el-form :model="form" label-width="180px" style="max-width:800px">
        <el-divider>nginx 全局参数</el-divider>
        <el-form-item label="工作进程数">
          <el-input v-model="form.nginx_worker_processes" placeholder="auto">
            <template #append>worker_processes</template>
          </el-input>
        </el-form-item>
        <el-form-item label="最大并发连接数">
          <el-input v-model="form.nginx_worker_connections">
            <template #append>worker_connections</template>
          </el-input>
        </el-form-item>
        <el-form-item label="长连接超时时间">
          <el-input v-model="form.nginx_keepalive_timeout">
            <template #append>秒 · keepalive_timeout</template>
          </el-input>
        </el-form-item>
        <el-form-item label="最大请求体大小">
          <el-input v-model="form.nginx_client_max_body_size" placeholder="64m">
            <template #append>client_max_body_size</template>
          </el-input>
        </el-form-item>
        <el-form-item label="默认日志轮转大小">
          <el-input v-model="form.default_log_max_size" placeholder="5M">
            <template #append>超过此大小自动压缩</template>
          </el-input>
        </el-form-item>

        <el-divider>SSL 自动续签（Let's Encrypt + DNSPod）</el-divider>
        <el-alert type="info" :closable="false" style="margin-bottom:16px;max-width:640px">
          使用 <b>Let's Encrypt</b> 免费证书，通过 DNSPod API 自动完成 DNS-01 验证。<br>
          支持两种 API：<b>DNSPod Token API</b>（推荐，在 dnspod.cn 控制台生成）或腾讯云 CAM SecretId/Key。填其中一种即可。
        </el-alert>
        <el-form-item label="ACME 邮箱" required>
          <el-input v-model="form.acme_email" placeholder="your@email.com（用于 Let's Encrypt 账号注册）" />
        </el-form-item>

        <el-divider content-position="left"><span style="font-size:13px;color:#909399">方式一：DNSPod Token API（推荐）</span></el-divider>
        <el-form-item label="DNSPod ID">
          <el-input v-model="form.dnspod_id" placeholder="DNSPod 用户中心 → API Token → ID" style="max-width:200px" />
          <span style="margin-left:8px;color:#999;font-size:12px">在 <a href="https://console.dnspod.cn/account/token/token" target="_blank">dnspod.cn</a> 生成</span>
        </el-form-item>
        <el-form-item label="DNSPod Token Key">
          <el-input v-model="form.dnspod_key" type="password" show-password placeholder="未修改保持为空" />
        </el-form-item>

        <el-divider content-position="left"><span style="font-size:13px;color:#909399">方式二：腾讯云 CAM API</span></el-divider>
        <el-form-item label="CAM SecretId">
          <el-input v-model="form.tencent_secret_id" placeholder="腾讯云控制台 → 访问管理 → API 密钥" />
        </el-form-item>
        <el-form-item label="CAM SecretKey">
          <el-input v-model="form.tencent_secret_key" type="password" show-password placeholder="未修改保持为空" />
        </el-form-item>

        <el-divider>邮件通知</el-divider>
        <el-form-item label="邮箱服务商">
          <el-select v-model="smtpProvider" style="width:160px" @change="onProviderChange">
            <el-option v-for="p in smtpPresets" :key="p.value" :label="p.name" :value="p.value" />
            <el-option label="自定义" value="custom" />
          </el-select>
          <span v-if="smtpProvider && smtpProvider !== 'custom'" style="margin-left:12px;font-size:12px;color:#909399">
            已自动填入 {{ currentPreset?.host }}:{{ currentPreset?.port }} · {{ currentPreset?.tls ? 'SSL/TLS' : 'STARTTLS' }}
          </span>
        </el-form-item>
        <el-form-item label="用户名（邮箱地址）">
          <el-input v-model="form.smtp_user" :placeholder="currentPreset?.userPlaceholder || 'user@example.com'" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.smtp_password" type="password" show-password
            :placeholder="currentPreset?.needAuthCode ? '授权码（非登录密码）' : '密码'" />
          <div v-if="currentPreset?.needAuthCode" style="color:#e6a23c;font-size:12px;margin-top:4px">
            ⚠️ 须填<b>授权码</b>，不是邮箱登录密码。<br>
            获取方式：网页登录邮箱 → 设置 → POP3/SMTP/IMAP → 开启 SMTP 服务 → 新增授权密码
          </div>
        </el-form-item>
        <el-form-item label="收件人地址">
          <el-input v-model="form.notify_email_to" placeholder="admin@example.com，多个用英文逗号分隔" />
        </el-form-item>
        <el-form-item label="">
          <el-button size="small" @click="testEmail" :loading="testingEmail">发送测试邮件</el-button>
        </el-form-item>
        <!-- 高级设置：自定义或微调时展开 -->
        <el-form-item label="">
          <el-collapse style="width:100%;border:none">
            <el-collapse-item title="高级设置（自定义 SMTP 服务器时展开）" name="adv">
              <el-form-item label="SMTP 服务器" style="margin-bottom:12px">
                <el-input v-model="form.smtp_host" placeholder="smtp.example.com" style="width:220px" />
                <el-input-number v-model.number="form.smtp_port" :min="1" :max="65535"
                  placeholder="端口" style="width:90px;margin-left:8px" />
                <el-switch v-model="smtpTLS" style="margin-left:12px"
                  active-text="SSL/TLS" inactive-text="STARTTLS"
                  @change="v => form.smtp_tls = v ? '1' : '0'" />
              </el-form-item>
              <el-form-item label="发件人显示名" style="margin-bottom:0">
                <el-input v-model="form.smtp_from" placeholder="NginxFlow <noreply@example.com>" />
                <div style="color:#999;font-size:12px;margin-top:4px">留空则使用用户名。注意：163/126 信封发件人固定为用户名，此字段仅影响显示</div>
              </el-form-item>
            </el-collapse-item>
          </el-collapse>
        </el-form-item>

        <el-divider>通知类型</el-divider>
        <el-form-item label="证书续签失败">
          <el-switch v-model="form.notify_cert_fail" active-value="1" inactive-value="0" />
          <span style="margin-left:12px;color:#999;font-size:12px">证书自动或手动续签失败时通知</span>
        </el-form-item>
        <el-form-item label="证书续签成功">
          <el-switch v-model="form.notify_cert_success" active-value="1" inactive-value="0" />
          <span style="margin-left:12px;color:#999;font-size:12px">证书续签完成并生效时通知</span>
        </el-form-item>
        <el-form-item label="节点下线告警">
          <el-switch v-model="form.notify_server_down" active-value="1" inactive-value="0" />
          <span style="margin-left:12px;color:#999;font-size:12px">后端节点健康检查失败下线时通知</span>
        </el-form-item>
        <el-form-item label="节点恢复通知">
          <el-switch v-model="form.notify_server_up" active-value="1" inactive-value="0" />
          <span style="margin-left:12px;color:#999;font-size:12px">下线节点重新上线时通知</span>
        </el-form-item>

        <el-divider>主从同步</el-divider>
        <el-alert type="info" :closable="false" style="margin-bottom:16px;max-width:620px">
          <b>主节点</b>：设置「同步鉴权 Token」，在「从节点」页面查看同步状态。<br>
          <b>从节点</b>：填写主节点地址和 Token，本机将定时拉取主节点的规则、证书、通知配置并自动应用。从节点不执行 SSL 续签。
        </el-alert>
        <el-form-item label="同步鉴权 Token">
          <el-input v-model="form.sync_token" type="password" show-password placeholder="主节点：设置此 token 供从节点鉴权" style="max-width:360px" />
        </el-form-item>
        <el-form-item label="主节点地址">
          <el-input v-model="form.slave_master_url" placeholder="http://10.x.x.x:9000（留空=本机是主节点）" />
          <div style="color:#999;font-size:12px;margin-top:4px">填写后本机进入从节点模式，自动定时同步</div>
        </el-form-item>
        <el-form-item label="主节点 Token">
          <el-input v-model="form.slave_sync_token" type="password" show-password placeholder="主节点的同步鉴权 Token" />
        </el-form-item>
        <el-form-item label="同步间隔（秒）">
          <el-input-number v-model.number="form.slave_interval" :min="10" :max="3600" placeholder="60" />
          <span style="margin-left:12px;color:#999;font-size:12px">从节点每隔此秒数检测主节点是否有更新</span>
        </el-form-item>
        <el-form-item v-if="slaveStatus" label="从节点状态">
          <el-tag :type="form.slave_last_status==='ok'?'success':'danger'" size="small">{{ form.slave_last_status==='ok'?'正常':'异常' }}</el-tag>
          <span style="margin-left:8px;color:#999;font-size:12px">{{ form.slave_last_sync_at }} — {{ form.slave_last_msg }}</span>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="save">保存</el-button>
          <el-button @click="load">重置</el-button>
          <el-divider direction="vertical" />
          <el-button @click="testNginx">测试 nginx 配置</el-button>
          <el-button @click="reloadNginx">重载 nginx</el-button>
          <el-button type="info" @click="backup">导出备份</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const form = ref({})
const testingEmail = ref(false)
const smtpProvider = ref('')

const smtpPresets = [
  { value: '163',     name: '163邮箱',   host: 'smtp.163.com',       port: 465, tls: true,  needAuthCode: true,  userPlaceholder: 'yourname@163.com' },
  { value: '126',     name: '126邮箱',   host: 'smtp.126.com',       port: 465, tls: true,  needAuthCode: true,  userPlaceholder: 'yourname@126.com' },
  { value: 'yeah',    name: 'yeah邮箱',  host: 'smtp.yeah.net',      port: 465, tls: true,  needAuthCode: true,  userPlaceholder: 'yourname@yeah.net' },
  { value: 'qq',      name: 'QQ邮箱',    host: 'smtp.qq.com',        port: 465, tls: true,  needAuthCode: true,  userPlaceholder: 'yourQQ@qq.com' },
  { value: 'gmail',   name: 'Gmail',     host: 'smtp.gmail.com',     port: 465, tls: true,  needAuthCode: true,  userPlaceholder: 'you@gmail.com' },
  { value: 'outlook', name: 'Outlook',   host: 'smtp.office365.com', port: 587, tls: false, needAuthCode: false, userPlaceholder: 'you@outlook.com' },
  { value: 'qq_ent',  name: '腾讯企业邮', host: 'smtp.exmail.qq.com', port: 465, tls: true,  needAuthCode: false, userPlaceholder: 'you@yourcompany.com' },
  { value: 'ali',     name: '阿里企业邮', host: 'smtp.qiye.aliyun.com', port: 465, tls: true, needAuthCode: false, userPlaceholder: 'you@yourcompany.com' },
]

const currentPreset = computed(() => smtpPresets.find(p => p.value === smtpProvider.value) || null)

function onProviderChange(val) {
  const p = smtpPresets.find(x => x.value === val)
  if (!p) return
  form.value.smtp_host = p.host
  form.value.smtp_port = p.port
  form.value.smtp_tls  = p.tls ? '1' : '0'
}

function detectProvider(host) {
  if (!host) return ''
  const p = smtpPresets.find(x => x.host === host)
  return p ? p.value : 'custom'
}

const slaveStatus = computed(() => !!form.value.slave_last_sync_at)

const smtpTLS = computed({
  get: () => form.value.smtp_tls !== '0',
  set: (v) => { form.value.smtp_tls = v ? '1' : '0' }
})

async function load() {
  form.value = (await api.get('/settings')).data
  if (form.value.smtp_port) form.value.smtp_port = Number(form.value.smtp_port)
  if (form.value.slave_interval) form.value.slave_interval = Number(form.value.slave_interval)
  smtpProvider.value = detectProvider(form.value.smtp_host)
}

async function save() {
  const data = {}
  for (const k in form.value) {
    if (form.value[k] !== '***') data[k] = String(form.value[k] ?? '')
  }
  await api.put('/settings', data)
  ElMessage.success('已保存')
  load()
}

async function testEmail() {
  testingEmail.value = true
  try {
    await api.post('/settings/test_email')
    ElMessage.success('测试邮件已发送，请检查收件箱')
  } catch (e) {
    ElMessage.error('发送失败：' + (e?.response?.data?.msg || e.message || '未知错误'))
  }
  testingEmail.value = false
}

async function testNginx() {
  try {
    const res = await api.post('/settings/nginx_test')
    ElMessageBox.alert(res.data.output, 'nginx 语法检查', { type: 'success' })
  } catch (e) {
    ElMessageBox.alert(e.msg || '失败', 'nginx 语法错误', { type: 'error' })
  }
}
async function reloadNginx() {
  await api.post('/settings/nginx_reload')
  ElMessage.success('nginx 已重载')
}
async function backup() {
  try {
    const blob = await api.get('/settings/backup', { responseType: 'blob' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `nginxflow-backup-${new Date().toISOString().slice(0,10)}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    ElMessage.success('备份已下载')
  } catch {}
}
onMounted(load)
</script>

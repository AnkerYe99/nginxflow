<template>
  <el-container class="layout">
    <div class="sidebar-mask" v-if="sideOpen && isMobile" @click="sideOpen=false" />
    <el-aside :width="isMobile ? '220px' : '220px'"
      :class="['sidebar', { 'sidebar-open': sideOpen || !isMobile }]">
      <div class="logo">
        <el-icon :size="22" color="#409EFF"><Promotion /></el-icon>
        <span>NginxFlow</span>
      </div>
      <el-menu :default-active="$route.path" router @select="isMobile && (sideOpen=false)">
        <el-menu-item index="/dashboard"><el-icon><DataAnalysis/></el-icon><span>总览</span></el-menu-item>
        <el-menu-item index="/rules"><el-icon><Connection/></el-icon><span>转发规则</span></el-menu-item>
        <el-menu-item index="/servers"><el-icon><Monitor/></el-icon><span>节点监控</span></el-menu-item>
        <el-menu-item index="/certs"><el-icon><Lock/></el-icon><span>SSL证书</span></el-menu-item>
        <el-menu-item index="/sync"><el-icon><Share/></el-icon><span>从节点</span></el-menu-item>
        <el-menu-item index="/settings"><el-icon><Setting/></el-icon><span>系统设置</span></el-menu-item>
      </el-menu>
    </el-aside>
    <el-container class="main-wrap">
      <el-header class="header">
        <el-icon v-if="isMobile" class="hamburger" :size="22" @click="sideOpen=!sideOpen"><Menu /></el-icon>
        <span v-else></span>
        <el-dropdown @command="handleDropdown">
          <span class="user-info">
            <el-icon><User/></el-icon>
            {{ username }}
            <el-icon><ArrowDown/></el-icon>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item disabled style="font-size:12px;color:#999">
                无操作 30 分钟自动退出
              </el-dropdown-item>
              <el-dropdown-item command="profile" divided>账号设置</el-dropdown-item>
              <el-dropdown-item command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main><router-view /></el-main>
      <el-footer class="footer" height="36px">
        <a href="mailto:AnkerYe@gmail.com">Copyright © AnkerYe. All rights reserved.</a>
      </el-footer>
    </el-container>
  </el-container>

  <!-- 账号设置对话框 -->
  <el-dialog v-model="profileShow" title="账号设置" width="420px" :close-on-click-modal="false">
    <el-form :model="profileForm" label-width="90px">
      <el-form-item label="新用户名">
        <el-input v-model="profileForm.username" :placeholder="`当前: ${username}`" clearable />
        <div style="color:#999;font-size:12px;margin-top:4px">留空则不修改</div>
      </el-form-item>
      <el-form-item label="当前密码" required>
        <el-input v-model="profileForm.old_password" type="password" show-password />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input v-model="profileForm.new_password" type="password" show-password placeholder="留空则不修改，至少6位" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="profileShow=false">取消</el-button>
      <el-button type="primary" :loading="profileSaving" @click="saveProfile">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { touchActivity } from '../api'
import api from '../api'

const IDLE_TIMEOUT = 30 * 60 * 1000

const router = useRouter()
const username = ref(localStorage.getItem('username') || 'admin')
const sideOpen = ref(false)
const isMobile = ref(false)

const profileShow = ref(false)
const profileSaving = ref(false)
const profileForm = ref({ username: '', old_password: '', new_password: '' })

function handleDropdown(cmd) {
  if (cmd === 'logout') logout()
  else if (cmd === 'profile') {
    profileForm.value = { username: '', old_password: '', new_password: '' }
    profileShow.value = true
  }
}

async function saveProfile() {
  if (!profileForm.value.old_password) return ElMessage.warning('请输入当前密码')
  profileSaving.value = true
  try {
    await api.put('/auth/profile', profileForm.value)
    ElMessage.success('修改成功，请重新登录')
    if (profileForm.value.username) {
      localStorage.setItem('username', profileForm.value.username)
      username.value = profileForm.value.username
    }
    profileShow.value = false
    setTimeout(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('lastActivity')
      router.push('/login')
    }, 1200)
  } catch {}
  profileSaving.value = false
}

function checkMobile() { isMobile.value = window.innerWidth < 768 }
function onActivity() { touchActivity() }

let idleTimer = null
function startIdleCheck() {
  idleTimer = setInterval(() => {
    const last = parseInt(localStorage.getItem('lastActivity') || '0')
    if (last && Date.now() - last > IDLE_TIMEOUT) {
      clearInterval(idleTimer)
      localStorage.removeItem('token')
      localStorage.removeItem('lastActivity')
      ElMessage.warning('已因长时间无操作自动退出')
      router.push('/login')
    }
  }, 60 * 1000)
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
  window.addEventListener('mousemove', onActivity)
  window.addEventListener('keydown', onActivity)
  window.addEventListener('click', onActivity)
  touchActivity()
  startIdleCheck()
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
  window.removeEventListener('mousemove', onActivity)
  window.removeEventListener('keydown', onActivity)
  window.removeEventListener('click', onActivity)
  clearInterval(idleTimer)
})

function logout() {
  clearInterval(idleTimer)
  localStorage.removeItem('token')
  localStorage.removeItem('lastActivity')
  router.push('/login')
}
</script>

<style scoped>
.layout { height: 100vh; overflow: hidden; }
.sidebar { background: #001529; color: #fff; height: 100vh; flex-shrink: 0;
  display: flex; flex-direction: column; transition: transform 0.25s; }
.logo { display: flex; align-items: center; gap: 10px; padding: 16px;
  font-size: 17px; font-weight: bold; color: #fff; border-bottom: 1px solid #112; flex-shrink: 0; }
.el-menu { border-right: none !important; background: #001529; flex: 1; }
:deep(.el-menu-item) { color: #c8ced4; }
:deep(.el-menu-item.is-active) { background: #1890ff !important; color: #fff !important; }
:deep(.el-menu-item:hover) { background: #112240 !important; }
.main-wrap { display: flex; flex-direction: column; overflow: hidden; }
.header { background: #fff; display: flex; justify-content: space-between; align-items: center;
  box-shadow: 0 1px 4px rgba(0,21,41,.08); flex-shrink: 0; }
.user-info { display: flex; align-items: center; gap: 6px; cursor: pointer; color: #606266; }
.hamburger { cursor: pointer; color: #333; }
.el-main { background: #f0f2f5; padding: 16px; overflow-y: auto; flex: 1; }
.footer { background: #fff; display: flex; align-items: center; justify-content: center;
  border-top: 1px solid #f0f0f0; }
.footer a { font-size: 12px; color: #999; text-decoration: none; }
.footer a:hover { color: #409EFF; }
.sidebar-mask { position: fixed; inset: 0; background: rgba(0,0,0,.45); z-index: 99; }

@media (max-width: 767px) {
  .sidebar { position: fixed; left: 0; top: 0; z-index: 100; transform: translateX(-100%); }
  .sidebar.sidebar-open { transform: translateX(0); }
}
</style>

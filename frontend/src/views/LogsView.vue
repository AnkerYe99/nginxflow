<template>
  <div>
    <!-- 头部：筛选 + 刷新 -->
    <div class="log-header">
      <div class="log-filters">
        <h2 class="log-title">日志管理</h2>
        <el-select v-model="filterType" placeholder="全部类型" clearable size="small"
          class="filter-select" @change="resetPage">
          <el-option label="访问日志" value="access" />
          <el-option label="错误日志" value="error" />
          <el-option label="捕获日志" value="capture" />
          <el-option label="流量日志" value="stream" />
        </el-select>
        <el-select v-model="filterRule" placeholder="全部规则" clearable size="small"
          class="filter-select filter-select-rule" @change="resetPage">
          <el-option v-for="r in ruleOptions" :key="r.id" :label="r.name" :value="r.id" />
        </el-select>
      </div>
      <el-button size="small" @click="load" :loading="loading">刷新</el-button>
    </div>

    <!-- 表格 -->
    <el-card shadow="never" :body-style="{padding:'0'}">
      <el-table :data="pagedData" v-loading="loading" size="small" stripe
        :scroll-x="true" style="width:100%">
        <el-table-column label="文件名" prop="name" min-width="200">
          <template #default="{row}">
            <el-icon style="color:#409eff;margin-right:4px;vertical-align:-2px"><Document /></el-icon>
            <span class="mono">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="规则" prop="rule_name" min-width="110">
          <template #default="{row}">
            <el-tag size="small" type="info" v-if="row.rule_name">{{ row.rule_name }}</el-tag>
            <span v-else class="dim">规则 {{ row.rule_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="类型" prop="type" min-width="72">
          <template #default="{row}">
            <el-tag size="small" :type="typeColor(row.type)">{{ typeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="大小" prop="size_human" min-width="84" align="right" />
        <el-table-column label="修改时间" prop="mod_time" min-width="148" />
        <el-table-column label="操作" min-width="150" fixed="right">
          <template #default="{row}">
            <el-button size="small" link type="primary" @click="viewFile(row)">查看</el-button>
            <el-button size="small" link type="primary" @click="download(row)">下载</el-button>
            <el-button size="small" link type="danger" @click="confirmDelete(row)">清空</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-wrap" v-if="filtered.length > pageSize">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="filtered.length"
          layout="total, prev, pager, next"
          background
          small
        />
      </div>
    </el-card>

    <!-- 查看日志对话框 -->
    <el-dialog v-model="viewVisible" :title="viewTitle"
      :width="isMobile ? '96vw' : '900px'" top="4vh"
      :close-on-click-modal="false" destroy-on-close>
      <div class="view-toolbar">
        <span class="dim" style="font-size:12px">最后 {{ viewLines }} 行</span>
        <div style="display:flex;gap:8px;align-items:center">
          <el-select v-model="viewLines" size="small" style="width:96px"
            @change="loadView(viewFile_)">
            <el-option :value="100" label="100 行" />
            <el-option :value="200" label="200 行" />
            <el-option :value="500" label="500 行" />
            <el-option :value="1000" label="1000 行" />
            <el-option :value="2000" label="2000 行" />
          </el-select>
          <el-button size="small" @click="loadView(viewFile_)" :loading="viewLoading">刷新</el-button>
        </div>
      </div>
      <el-input
        v-model="viewContent"
        type="textarea"
        :rows="isMobile ? 18 : 28"
        readonly
        class="log-textarea"
        resize="none"
      />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const loading = ref(false)
const list = ref([])
const filterType = ref('')
const filterRule = ref(null)
const ruleOptions = ref([])
const currentPage = ref(1)
const pageSize = 30
const isMobile = ref(window.innerWidth < 768)

function onResize() { isMobile.value = window.innerWidth < 768 }
onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))

const filtered = computed(() => {
  return list.value.filter(f => {
    if (filterType.value && f.type !== filterType.value) return false
    if (filterRule.value && f.rule_id !== filterRule.value) return false
    return true
  })
})

const pagedData = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filtered.value.slice(start, start + pageSize)
})

function resetPage() { currentPage.value = 1 }

watch(filtered, resetPage)

async function load() {
  loading.value = true
  try {
    const res = await api.get('/logs')
    list.value = res.data || []
    const seen = new Set()
    ruleOptions.value = []
    for (const f of list.value) {
      if (!seen.has(f.rule_id)) {
        seen.add(f.rule_id)
        ruleOptions.value.push({ id: f.rule_id, name: f.rule_name || `规则 ${f.rule_id}` })
      }
    }
    ruleOptions.value.sort((a, b) => a.id - b.id)
  } catch {
    ElMessage.error('获取日志列表失败')
  }
  loading.value = false
}

// ---------- 查看 ----------
const viewVisible = ref(false)
const viewTitle = ref('')
const viewContent = ref('')
const viewLines = ref(500)
const viewLoading = ref(false)
const viewFile_ = ref(null)

async function viewFile(row) {
  viewFile_.value = row
  viewTitle.value = row.name
  viewVisible.value = true
  await loadView(row)
}

async function loadView(row) {
  if (!row) return
  viewLoading.value = true
  try {
    const res = await api.get('/logs/view', { params: { file: row.name, lines: viewLines.value } })
    viewContent.value = res.data.content || '(空文件)'
  } catch {
    viewContent.value = '(读取失败)'
  }
  viewLoading.value = false
}

// ---------- 下载 ----------
function download(row) {
  const token = localStorage.getItem('token')
  const base = api.defaults.baseURL || ''
  const url = `${base}/logs/download?file=${encodeURIComponent(row.name)}`
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then(r => r.blob())
    .then(blob => {
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = row.name
      a.click()
      URL.revokeObjectURL(a.href)
    })
    .catch(() => ElMessage.error('下载失败'))
}

// ---------- 清空 ----------
async function confirmDelete(row) {
  try {
    await ElMessageBox.confirm(
      `确定清空 <b>${row.name}</b> 吗？（文件保留，内容清零，nginx 继续写入）`,
      '清空日志',
      { confirmButtonText: '清空', cancelButtonText: '取消', type: 'warning', dangerouslyUseHTMLString: true }
    )
  } catch { return }
  try {
    await api.delete('/logs', { params: { file: row.name } })
    ElMessage.success('已清空')
    load()
  } catch {
    ElMessage.error('清空失败')
  }
}

function typeLabel(t) {
  return { access: '访问', error: '错误', capture: '捕获', stream: '流量' }[t] || t
}
function typeColor(t) {
  return { access: '', error: 'danger', capture: 'warning', stream: 'info' }[t] || ''
}

onMounted(load)
</script>

<style scoped>
.log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  gap: 8px;
  flex-wrap: wrap;
}
.log-filters {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.log-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
}
.filter-select { width: 120px; }
.filter-select-rule { width: 150px; }
.mono { font-family: monospace; font-size: 12px; }
.dim { color: #aaa; font-size: 12px; }

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid #f0f0f0;
}

.view-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
  gap: 8px;
}
.log-textarea :deep(textarea) {
  font-family: monospace;
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 767px) {
  .log-header { flex-direction: column; align-items: flex-start; }
  .filter-select { width: 110px; }
  .filter-select-rule { width: 130px; }
  .pagination-wrap { justify-content: center; padding: 10px 8px; }
  .pagination-wrap :deep(.el-pagination) { flex-wrap: wrap; justify-content: center; }
}
</style>

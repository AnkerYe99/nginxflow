<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;flex-wrap:wrap;gap:8px">
      <h2 style="margin:0">节点健康状态</h2>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
        <el-button size="small" :type="sortByReq?'primary':'default'" @click="sortByReq=!sortByReq">
          {{ sortByReq ? '▼ 浏览量排序' : '默认排序' }}
        </el-button>
        <el-button size="small" :icon="Refresh" @click="load" :loading="loading">刷新</el-button>
        <el-switch v-model="autoRefresh" active-text="自动刷新" @change="toggleAuto" />
      </div>
    </div>

    <el-card shadow="never" v-loading="loading">
      <div style="overflow-x:auto">
      <el-table :data="pagedRows" size="small" :span-method="spanMethod"
        :row-class-name="rowClassName" border style="width:100%" table-layout="auto">
        <el-table-column label="规则名" prop="rule_name" min-width="120">
          <template #default="{row}">
            <div class="rule-cell">
              <span class="rule-cell-name">{{ row.rule_name }}</span>
              <el-tag v-if="ruleDownMap[row.rule_id]>0" type="danger" size="small" effect="plain" style="margin-left:6px">
                {{ ruleDownMap[row.rule_id] }} 下线
              </el-tag>
              <el-tag v-else type="success" size="small" effect="plain" style="margin-left:6px">健康</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="节点地址" min-width="170">
          <template #default="{row}">
            <span class="mono">{{ row.address }}:{{ row.port }}</span>
            <span class="weight-badge">w{{ row.weight }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" min-width="74" align="center">
          <template #default="{row}">
            <el-tag v-if="row.state==='up'" type="success" size="small" effect="dark">正常</el-tag>
            <el-tag v-else-if="row.state==='down'" type="danger" size="small" effect="dark">异常</el-tag>
            <el-tag v-else type="info" size="small">禁用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="今天访问" min-width="86" align="right">
          <template #default="{row}">
            <span :class="row.today_req?'num-active':'num-zero'">{{ fmtNum(row.today_req) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="今天流量" min-width="96" align="right">
          <template #default="{row}">
            <span :class="row.today_bytes?'num-active':'num-zero'">{{ fmtBytes(row.today_bytes) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="7天访问" min-width="86" align="right">
          <template #default="{row}">
            <span :class="row.d7_req?'num-active':'num-zero'">{{ fmtNum(row.d7_req) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="7天流量" min-width="96" align="right">
          <template #default="{row}">
            <span :class="row.d7_bytes?'num-active':'num-zero'">{{ fmtBytes(row.d7_bytes) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="30天访问" min-width="86" align="right">
          <template #default="{row}">
            <span :class="row.d30_req?'num-active':'num-zero'">{{ fmtNum(row.d30_req) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="30天流量" min-width="96" align="right">
          <template #default="{row}">
            <span :class="row.d30_bytes?'num-active':'num-zero'">{{ fmtBytes(row.d30_bytes) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="最后检测" min-width="148" align="center">
          <template #default="{row}">
            <span style="color:#909399;font-size:12px">{{ row.last_check_at || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="错误信息" min-width="120">
          <template #default="{row}">
            <span v-if="row.last_err" class="err-text" :title="row.last_err">⚠ {{ row.last_err.slice(0,50) }}</span>
          </template>
        </el-table-column>
      </el-table>
      </div>

      <div v-if="!loading && sortedServerHealth.length===0" class="empty-tip">暂无节点数据</div>
      <Pagination :total="sortedServerHealth.length" :page-size="PAGE_SIZE" v-model:current="page" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import api from '../api'
import Pagination from '../components/Pagination.vue'

const PAGE_SIZE = 30
const serverHealth = ref([])
const loading = ref(false)
const sortByReq = ref(true)
const autoRefresh = ref(false)
const page = ref(1)
let timer = null

async function load() {
  loading.value = true
  try {
    const res = await api.get('/stats/server_health')
    serverHealth.value = res.data || []
  } catch {}
  loading.value = false
}

function toggleAuto(val) {
  clearInterval(timer)
  if (val) timer = setInterval(load, 15000)
}

// 按规则分组后排序展开的全量列表
const sortedServerHealth = computed(() => {
  const raw = serverHealth.value
  if (!sortByReq.value) return raw
  const groups = []
  const seen = {}
  for (const r of raw) {
    if (!seen[r.rule_id]) { seen[r.rule_id] = []; groups.push(seen[r.rule_id]) }
    seen[r.rule_id].push(r)
  }
  groups.sort((a, b) =>
    b.reduce((s, r) => s + r.today_req, 0) - a.reduce((s, r) => s + r.today_req, 0)
  )
  return groups.flat()
})

// 当前页数据
const pagedRows = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return sortedServerHealth.value.slice(start, start + PAGE_SIZE)
})

// 每条规则的下线节点数（基于全量数据）
const ruleDownMap = computed(() => {
  const m = {}
  for (const r of serverHealth.value) {
    if (!m[r.rule_id]) m[r.rule_id] = 0
    if (r.state === 'down') m[r.rule_id]++
  }
  return m
})

// span-method 基于当前页行计算，规则名列按规则分组合并
const spanMap = computed(() => {
  const map = {}
  const rows = pagedRows.value
  let i = 0
  while (i < rows.length) {
    const ruleId = rows[i].rule_id
    let j = i
    while (j < rows.length && rows[j].rule_id === ruleId) j++
    map[i] = { rowspan: j - i, colspan: 1 }
    for (let k = i + 1; k < j; k++) map[k] = { rowspan: 0, colspan: 0 }
    i = j
  }
  return map
})

function spanMethod({ rowIndex, columnIndex }) {
  if (columnIndex === 0) return spanMap.value[rowIndex] || { rowspan: 0, colspan: 0 }
  return { rowspan: 1, colspan: 1 }
}

function rowClassName({ row }) {
  return row.state === 'down' ? 'row-down' : ''
}

function fmtBytes(b) {
  if (!b) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0; let v = b
  while (v >= 1024 && i < 4) { v /= 1024; i++ }
  return v.toFixed(1) + ' ' + units[i]
}
function fmtNum(n) {
  if (!n) return '0'
  if (n >= 1e8) return (n / 1e8).toFixed(1) + '亿'
  if (n >= 1e4) return (n / 1e4).toFixed(1) + '万'
  return n.toLocaleString()
}

onMounted(() => { load(); timer = setInterval(load, 15000) })
onUnmounted(() => clearInterval(timer))
</script>

<style scoped>
.rule-cell { display: flex; align-items: center; }
.rule-cell-name { font-weight: 600; color: #303133; font-size: 13px; }
.mono { font-family: monospace; color: #303133; }
.weight-badge { font-size: 10px; color: #c0c4cc; background: #f5f5f5;
  border-radius: 3px; padding: 0 4px; margin-left: 6px; line-height: 18px; }
.num-active { color: #303133; font-weight: 500; }
.num-zero { color: #c0c4cc; }
.err-text { color: #f56c6c; font-size: 12px; }
:deep(.row-down td) { background: #fff5f5 !important; }
.empty-tip { text-align: center; color: #c0c4cc; padding: 20px 0; font-size: 13px; }
</style>

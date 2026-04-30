<template>
  <div class="deploy-status">
    <div class="status-header">
      <h3>部署任务状态</h3>
      <span :class="['status-badge', statusClass]">{{ statusLabel }}</span>
    </div>

    <div v-if="taskStatus" class="status-details">
      <div class="detail-row">
        <span class="detail-label">项目</span>
        <span class="detail-value">{{ taskStatus.project_name }}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">分支</span>
        <span class="detail-value">{{ taskStatus.branch }}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">环境</span>
        <span class="detail-value">{{ environmentLabel }}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">创建时间</span>
        <span class="detail-value">{{ formatTime(taskStatus.created_at) }}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">更新时间</span>
        <span class="detail-value">{{ formatTime(taskStatus.updated_at) }}</span>
      </div>
    </div>

    <div v-if="errorMessage" class="status-error" role="alert">
      {{ errorMessage }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { fetchTaskStatus, type TaskStatus } from '../api/index'

const props = defineProps<{
  taskId: string
}>()

const emit = defineEmits<{
  complete: []
}>()

const taskStatus = ref<TaskStatus | null>(null)
const errorMessage = ref('')
let pollingTimer: ReturnType<typeof setInterval> | null = null

const STATUS_MAP: Record<string, { label: string; cssClass: string }> = {
  pending: { label: '等待中', cssClass: 'status-pending' },
  cloning: { label: '拉取代码中', cssClass: 'status-running' },
  building: { label: '构建中', cssClass: 'status-running' },
  deploying: { label: '部署中', cssClass: 'status-running' },
  success: { label: '成功', cssClass: 'status-success' },
  failed: { label: '失败', cssClass: 'status-failed' },
}

const ENVIRONMENT_LABELS: Record<string, string> = {
  dev: '开发环境',
  sit: '集成测试环境',
  prod: '生产环境',
}

const statusLabel = computed(() => {
  const status = taskStatus.value?.status
  if (!status) return '加载中'
  return STATUS_MAP[status]?.label ?? status
})

const statusClass = computed(() => {
  const status = taskStatus.value?.status
  if (!status) return 'status-pending'
  return STATUS_MAP[status]?.cssClass ?? 'status-pending'
})

const environmentLabel = computed(() => {
  const env = taskStatus.value?.environment
  if (!env) return ''
  return ENVIRONMENT_LABELS[env] ?? env
})

function formatTime(timeStr: string): string {
  if (!timeStr) return '-'
  try {
    const date = new Date(timeStr)
    return date.toLocaleString('zh-CN')
  } catch {
    return timeStr
  }
}

function isTerminalStatus(status: string): boolean {
  return status === 'success' || status === 'failed'
}

async function pollStatus() {
  try {
    const result = await fetchTaskStatus(props.taskId)
    taskStatus.value = result
    errorMessage.value = ''

    if (isTerminalStatus(result.status)) {
      stopPolling()
      emit('complete')
    }
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `获取任务状态失败：${message}`
  }
}

function startPolling() {
  pollingTimer = setInterval(pollStatus, 5000)
}

function stopPolling() {
  if (pollingTimer !== null) {
    clearInterval(pollingTimer)
    pollingTimer = null
  }
}

onMounted(async () => {
  await pollStatus()
  // Only start polling if the task is not already in a terminal state
  if (taskStatus.value && !isTerminalStatus(taskStatus.value.status)) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.deploy-status {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 16px;
  margin-top: 16px;
  background: #fafafa;
}

.status-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.status-header h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
}

.status-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 0.85rem;
  font-weight: 500;
}

.status-pending {
  background: #f3f4f6;
  color: #6b7280;
}

.status-running {
  background: #dbeafe;
  color: #1d4ed8;
}

.status-success {
  background: #dcfce7;
  color: #166534;
}

.status-failed {
  background: #fee2e2;
  color: #b91c1c;
}

.status-details {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.9rem;
}

.detail-label {
  color: #6b7280;
  min-width: 72px;
  flex-shrink: 0;
}

.detail-value {
  color: #1a1a1a;
}

.status-error {
  margin-top: 12px;
  padding: 8px 12px;
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  color: #b91c1c;
  font-size: 0.85rem;
}
</style>

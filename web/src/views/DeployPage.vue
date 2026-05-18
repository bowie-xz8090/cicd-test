<template>
  <div class="deploy-page">
    <h1>部署操作</h1>
    <p class="subtitle">选择项目、分支和环境后触发部署。</p>

    <!-- Error alert -->
    <div v-if="errorMessage" class="error-alert" role="alert">
      {{ errorMessage }}
      <button class="error-close" @click="errorMessage = ''" aria-label="关闭错误提示">&times;</button>
    </div>

    <!-- Success message -->
    <div v-if="deployResult && !showDeployStatus" class="success-alert" role="status">
      部署已触发，任务 ID：{{ deployResult.task_id }}，状态：{{ deployResult.status }}
    </div>

    <!-- Deploy status tracking -->
    <DeployStatus
      v-if="showDeployStatus && deployResult"
      :task-id="deployResult.task_id"
      @complete="onDeployComplete"
    />

    <form class="deploy-form" @submit.prevent="handleDeploy">
      <!-- Project select -->
      <div class="form-group">
        <label for="project-select">项目</label>
        <select
          id="project-select"
          v-model="selectedProject"
          :disabled="loadingProjects"
        >
          <option value="" disabled>请选择项目</option>
          <option
            v-for="project in projects"
            :key="project.full_name"
            :value="project.full_name"
          >
            {{ project.full_name }}
          </option>
        </select>
        <span v-if="loadingProjects" class="loading-hint">加载项目列表中…</span>
      </div>

      <!-- Ref type toggle + Branch/Tag select -->
      <div class="form-group">
        <label>引用类型</label>
        <div class="ref-type-toggle">
          <label class="ref-radio">
            <input type="radio" name="refType" value="branch" v-model="refType" />
            分支
          </label>
          <label class="ref-radio">
            <input type="radio" name="refType" value="tag" v-model="refType" />
            标签
          </label>
        </div>
      </div>

      <div class="form-group">
        <label for="ref-select">{{ refType === 'branch' ? '分支' : '标签' }}</label>
        <select
          id="ref-select"
          v-model="selectedRef"
          :disabled="!selectedProject || loadingRefs"
        >
          <option value="" disabled>{{ refType === 'branch' ? '请选择分支' : '请选择标签' }}</option>
          <option
            v-for="item in refList"
            :key="item.name"
            :value="item.name"
          >
            {{ item.name }}
          </option>
        </select>
        <span v-if="loadingRefs" class="loading-hint">加载中…</span>
      </div>

      <!-- Environment select -->
      <div class="form-group">
        <label id="env-label">环境</label>
        <div class="env-options" role="radiogroup" aria-labelledby="env-label">
          <label
            v-for="env in environments"
            :key="env.key"
            class="env-radio"
            :class="{ 'env-disabled': env.disabled }"
          >
            <input
              type="radio"
              name="environment"
              :value="env.key"
              v-model="selectedEnvironment"
              :disabled="env.disabled"
            />
            {{ env.label }}
            <span v-if="env.disabled" class="disabled-tag">未开放</span>
          </label>
        </div>
      </div>

      <!-- Deploy button -->
      <div class="form-group">
        <button
          type="submit"
          class="deploy-btn"
          :disabled="!canDeploy || deploying"
          :aria-disabled="!canDeploy || deploying"
          :title="deployButtonHint"
        >
          {{ deploying ? '部署中…' : '部署' }}
        </button>
        <span v-if="!canDeploy" class="hint">请先选择项目、{{ refType === 'branch' ? '分支' : '标签' }}和环境</span>
      </div>
    </form>

    <!-- Environment quick links -->
    <div v-if="environments.length > 0" class="env-links-section">
      <h3>环境访问</h3>
      <div class="env-links-grid">
        <div v-for="env in environments" :key="env.key" class="env-link-card">
          <span class="env-link-label">{{ env.label }}</span>
          <div class="env-link-buttons">
            <a
              v-if="env.user_url"
              :href="env.user_url"
              target="_blank"
              rel="noopener noreferrer"
              class="env-link-btn user-btn"
            >用户端</a>
            <a
              v-if="env.admin_url"
              :href="env.admin_url"
              target="_blank"
              rel="noopener noreferrer"
              class="env-link-btn admin-btn"
            >管理端</a>
            <template v-if="env.extra && env.extra.length > 0">
              <span class="env-link-divider">|</span>
              <a
                v-for="link in env.extra"
                :key="link.url"
                :href="link.url"
                target="_blank"
                rel="noopener noreferrer"
                class="env-link-btn extra-btn"
              >{{ link.label }}</a>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import {
  fetchProjects,
  fetchBranches,
  fetchTags,
  fetchEnvironments,
  triggerDeploy,
  type Project,
  type Branch,
  type Tag,
  type Environment,
  type DeployResponse,
} from '../api/index'
import DeployStatus from '../components/DeployStatus.vue'

const emit = defineEmits<{
  deployed: []
}>()

// --- Reactive state ---
const projects = ref<Project[]>([])
const branches = ref<Branch[]>([])
const tags = ref<Tag[]>([])
const environments = ref<Environment[]>([])

const selectedProject = ref('')
const refType = ref<'branch' | 'tag'>('branch')
const selectedRef = ref('')
const selectedEnvironment = ref('dev')

const errorMessage = ref('')
const deployResult = ref<DeployResponse | null>(null)

const loadingProjects = ref(false)
const loadingRefs = ref(false)
const deploying = ref(false)

const showDeployStatus = ref(false)

// --- Computed ---
const refList = computed(() => {
  return refType.value === 'branch' ? branches.value : tags.value
})

const canDeploy = computed(() => {
  return selectedProject.value !== '' && selectedRef.value !== '' && selectedEnvironment.value !== ''
})

const deployButtonHint = computed(() => {
  if (canDeploy.value) return '点击触发部署'
  const missing: string[] = []
  if (!selectedProject.value) missing.push('项目')
  if (!selectedRef.value) missing.push(refType.value === 'branch' ? '分支' : '标签')
  if (!selectedEnvironment.value) missing.push('环境')
  return `请先选择${missing.join('、')}`
})

// --- Load projects and environments on mount ---
onMounted(async () => {
  await Promise.all([loadProjects(), loadEnvironments()])
})

async function loadProjects() {
  loadingProjects.value = true
  try {
    projects.value = await fetchProjects()
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `获取项目列表失败：${message}`
  } finally {
    loadingProjects.value = false
  }
}

async function loadEnvironments() {
  try {
    environments.value = await fetchEnvironments()
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `获取环境列表失败：${message}`
  }
}

// --- Watch project selection to reload branches/tags ---
watch(selectedProject, async (newVal) => {
  selectedRef.value = ''
  branches.value = []
  tags.value = []
  deployResult.value = null

  if (!newVal) return
  await loadRefs()
})

// --- Watch refType to reload list ---
watch(refType, async () => {
  selectedRef.value = ''
  if (!selectedProject.value) return
  await loadRefs()
})

async function loadRefs() {
  const parts = selectedProject.value.split('/')
  if (parts.length < 2) return

  const owner = parts[0]
  const repo = parts.slice(1).join('/')

  loadingRefs.value = true
  try {
    if (refType.value === 'branch') {
      branches.value = await fetchBranches(owner, repo)
    } else {
      tags.value = await fetchTags(owner, repo)
    }
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `获取${refType.value === 'branch' ? '分支' : '标签'}列表失败：${message}`
  } finally {
    loadingRefs.value = false
  }
}

// --- Deploy handler ---
async function handleDeploy() {
  if (!canDeploy.value || deploying.value) return

  // 二次确认防呆
  const envLabel = environments.value.find(e => e.key === selectedEnvironment.value)?.label ?? selectedEnvironment.value
  const refLabel = refType.value === 'branch' ? '分支' : '标签'
  const confirmed = window.confirm(`确认部署？\n\n项目：${selectedProject.value}\n${refLabel}：${selectedRef.value}\n环境：${envLabel}`)
  if (!confirmed) return

  const parts = selectedProject.value.split('/')
  if (parts.length < 2) return

  const owner = parts[0]
  const repo = parts.slice(1).join('/')

  deploying.value = true
  errorMessage.value = ''
  deployResult.value = null
  showDeployStatus.value = false

  try {
    deployResult.value = await triggerDeploy({
      project_owner: owner,
      project_name: repo,
      branch: selectedRef.value,
      environment: selectedEnvironment.value,
    })
    showDeployStatus.value = true
    emit('deployed')
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `部署触发失败：${message}`
  } finally {
    deploying.value = false
  }
}

function onDeployComplete() {
  // Task finished (success or failed) — refresh history
  emit('deployed')
}
</script>

<style scoped>
.deploy-page {
  margin: 0;
  padding: 24px 16px;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  color: #1a1a1a;
}

h1 {
  font-size: 1.5rem;
  margin: 0 0 4px;
}

.subtitle {
  color: #666;
  margin: 0 0 24px;
  font-size: 0.95rem;
}

.error-alert {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  color: #b91c1c;
  padding: 12px 16px;
  border-radius: 6px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.9rem;
}

.error-close {
  background: none;
  border: none;
  color: #b91c1c;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 0 0 0 12px;
  line-height: 1;
}

.success-alert {
  background: #f0fdf4;
  border: 1px solid #86efac;
  color: #166534;
  padding: 12px 16px;
  border-radius: 6px;
  margin-bottom: 16px;
  font-size: 0.9rem;
}

.deploy-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group > label {
  font-weight: 600;
  font-size: 0.9rem;
}

select {
  padding: 8px 12px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: 0.95rem;
  background: #fff;
  color: #1a1a1a;
  appearance: auto;
}

select:disabled {
  background: #f3f4f6;
  color: #9ca3af;
  cursor: not-allowed;
}

.env-options {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.ref-type-toggle {
  display: flex;
  gap: 16px;
}

.ref-radio {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  font-weight: normal;
}

.ref-radio input[type='radio'] {
  margin: 0;
}

.env-radio {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.95rem;
  cursor: pointer;
  font-weight: normal;
}

.env-radio input[type='radio'] {
  margin: 0;
}

.env-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.env-disabled input[type='radio'] {
  cursor: not-allowed;
}

.disabled-tag {
  font-size: 0.75rem;
  color: #9ca3af;
  margin-left: 2px;
}

.deploy-btn {
  padding: 10px 24px;
  background: #2563eb;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
  align-self: flex-start;
}

.deploy-btn:hover:not(:disabled) {
  background: #1d4ed8;
}

.deploy-btn:disabled {
  background: #93c5fd;
  cursor: not-allowed;
}

.hint {
  color: #9ca3af;
  font-size: 0.85rem;
}

.loading-hint {
  color: #6b7280;
  font-size: 0.85rem;
}

.env-links-section {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid #e5e7eb;
}

.env-links-section h3 {
  font-size: 1rem;
  margin: 0 0 12px;
  font-weight: 600;
}

.env-links-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.env-link-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fafafa;
}

.env-link-label {
  font-size: 0.9rem;
  font-weight: 500;
  color: #374151;
  min-width: 72px;
}

.env-link-buttons {
  display: flex;
  gap: 8px;
}

.env-link-btn {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 500;
  text-decoration: none;
  transition: background 0.15s;
}

.user-btn {
  background: #dbeafe;
  color: #1d4ed8;
}

.user-btn:hover {
  background: #bfdbfe;
}

.admin-btn {
  background: #fef3c7;
  color: #92400e;
}

.admin-btn:hover {
  background: #fde68a;
}

.env-link-divider {
  color: #d1d5db;
  font-size: 0.9rem;
  margin: 0 2px;
}

.extra-btn {
  background: #e0e7ff;
  color: #3730a3;
}

.extra-btn:hover {
  background: #c7d2fe;
}
</style>

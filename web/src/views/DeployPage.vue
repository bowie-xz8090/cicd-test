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

      <!-- Branch select -->
      <div class="form-group">
        <label for="branch-select">分支</label>
        <select
          id="branch-select"
          v-model="selectedBranch"
          :disabled="!selectedProject || loadingBranches"
        >
          <option value="" disabled>请选择分支</option>
          <option
            v-for="branch in branches"
            :key="branch.name"
            :value="branch.name"
          >
            {{ branch.name }}
          </option>
        </select>
        <span v-if="loadingBranches" class="loading-hint">加载分支列表中…</span>
      </div>

      <!-- Environment select -->
      <div class="form-group">
        <label id="env-label">环境</label>
        <div class="env-options" role="radiogroup" aria-labelledby="env-label">
          <label
            v-for="env in environments"
            :key="env.key"
            class="env-radio"
          >
            <input
              type="radio"
              name="environment"
              :value="env.key"
              v-model="selectedEnvironment"
            />
            {{ env.label }}
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
        <span v-if="!canDeploy" class="hint">请先选择项目、分支和环境</span>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import {
  fetchProjects,
  fetchBranches,
  fetchEnvironments,
  triggerDeploy,
  type Project,
  type Branch,
  type Environment,
  type DeployResponse,
} from '../api/index'
import DeployStatus from '../components/DeployStatus.vue'

// --- Reactive state ---
const projects = ref<Project[]>([])
const branches = ref<Branch[]>([])
const environments = ref<Environment[]>([])

const selectedProject = ref('')
const selectedBranch = ref('')
const selectedEnvironment = ref('')

const errorMessage = ref('')
const deployResult = ref<DeployResponse | null>(null)

const loadingProjects = ref(false)
const loadingBranches = ref(false)
const deploying = ref(false)

const showDeployStatus = ref(false)

// --- Computed ---
const canDeploy = computed(() => {
  return selectedProject.value !== '' && selectedBranch.value !== '' && selectedEnvironment.value !== ''
})

const deployButtonHint = computed(() => {
  if (canDeploy.value) return '点击触发部署'
  const missing: string[] = []
  if (!selectedProject.value) missing.push('项目')
  if (!selectedBranch.value) missing.push('分支')
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

// --- Watch project selection to reload branches ---
watch(selectedProject, async (newVal) => {
  selectedBranch.value = ''
  branches.value = []
  deployResult.value = null

  if (!newVal) return

  const parts = newVal.split('/')
  if (parts.length < 2) return

  const owner = parts[0]
  const repo = parts.slice(1).join('/')

  loadingBranches.value = true
  try {
    branches.value = await fetchBranches(owner, repo)
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `获取分支列表失败：${message}`
  } finally {
    loadingBranches.value = false
  }
})

// --- Deploy handler ---
async function handleDeploy() {
  if (!canDeploy.value || deploying.value) return

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
      branch: selectedBranch.value,
      environment: selectedEnvironment.value,
    })
    showDeployStatus.value = true
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : '未知错误'
    errorMessage.value = `部署触发失败：${message}`
  } finally {
    deploying.value = false
  }
}

function onDeployComplete() {
  // Task finished (success or failed) — polling has stopped
}
</script>

<style scoped>
.deploy-page {
  max-width: 560px;
  margin: 0 auto;
  padding: 32px 16px;
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
</style>

<template>
  <div class="config-page">
    <div class="page-heading"><div><h1>配置管理</h1><p>修改后会立即应用到最新配置。</p></div></div>
    <div v-if="errorMessage" class="alert error-alert" role="alert">{{ errorMessage }}</div>
    <div v-if="successMessage" class="alert success-alert" role="status">{{ successMessage }}</div>
    <div v-if="loading" class="loading-state">加载配置中...</div>

    <template v-else>
      <section class="config-section">
        <div><h2>环境访问配置</h2><p>分别维护开发、测试和生产环境的访问入口。</p></div>
        <div class="environment-actions">
          <button v-for="environment in standardEnvironments" :key="environment.key" class="environment-btn" :disabled="!environments[environment.key]" @click="openEnvironmentDialog(environment.key)">{{ environment.label }}</button>
        </div>
      </section>

      <section class="config-section">
        <div class="section-heading"><div><h2>项目打包配置</h2><p>选择项目、子项目和环境后编辑打包及部署规则。</p></div><button class="save-btn" :disabled="savingProjects" @click="saveProjects">{{ savingProjects ? '保存中...' : '保存项目打包配置' }}</button></div>
        <div class="project-layout">
          <aside class="project-list">
            <button v-for="(_, key) in projects" :key="key" :class="['project-item', { active: selectedProjectKey === key }]" @click="selectProject(key)">{{ projects[key].label || key }}</button>
            <button class="add-btn add-project-btn" @click="openCreateDialog('project')">新增项目</button>
          </aside>

          <div v-if="selectedProject" class="project-editor">
            <div class="name-row"><div class="name-fields"><label>项目标识<input v-model.trim="projectKeyInput" @change="renameProject" /></label><label>项目名称<input v-model.trim="selectedProject.label" /></label></div><button class="remove-btn" @click="removeProject">删除项目</button></div>
            <div class="subproject-heading"><h3>子项目</h3><button class="add-btn" @click="openCreateDialog('subproject')">新增子项目</button></div>
            <div class="subproject-list"><button v-for="(_, key) in selectedProject.sub_projects" :key="key" :class="['subproject-item', { active: selectedSubProjectKey === key }]" @click="selectSubProject(key)">{{ selectedProject.sub_projects[key].label || key }}</button></div>

            <template v-if="selectedSubProject && currentEnvironmentConfig">
              <div class="name-row"><div class="name-fields"><label>子项目名称<input v-model.trim="selectedSubProject.label" /></label><label>构建类型<select v-model="selectedSubProject.build_type"><option value="">自动识别</option><option value="frontend">前端</option><option value="backend">后端</option></select></label></div><button class="remove-btn" @click="removeSubProject">删除子项目</button></div>
              <div class="environment-select"><label>环境<select v-model="selectedEnvironmentKey" @change="ensureEnvironmentConfig"><option value="dev">开发环境</option><option value="sit">测试环境</option><option value="prod">生产环境</option></select></label></div>
              <div class="build-form">
                <label class="switch-row"><span>部署权限</span><input :checked="currentEnvironmentConfig.disabled !== true" type="checkbox" @change="toggleProjectDeployPermission" /><span class="switch" aria-hidden="true"></span><span>{{ currentEnvironmentConfig.disabled === true ? '关闭' : '开启' }}</span></label>
                <label>构建命令<textarea v-model="currentEnvironmentConfig.build_cmd" rows="3" /></label>
                <label>部署服务器<select v-model="currentEnvironmentConfig.server"><option value="">请选择服务器</option><option v-for="server in serverOptions" :key="server" :value="server">{{ server }}</option></select></label>
                <label v-if="!hasArtifacts">构建产物目录<input v-model.trim="currentEnvironmentConfig.build_output" /></label>
                <label v-if="!hasArtifacts">产物重命名<input v-model.trim="currentEnvironmentConfig.rename_to" /></label>
                <label v-if="!hasArtifacts">部署脚本<textarea v-model="currentEnvironmentConfig.deploy_script" rows="6" /></label>
                <label class="switch-row"><span>多产物部署</span><input v-model="hasArtifacts" type="checkbox" /><span class="switch" aria-hidden="true"></span><span>{{ hasArtifacts ? '开启' : '关闭' }}</span></label>
                <div v-if="hasArtifacts" class="artifacts"><div class="section-heading"><h3>部署产物</h3><button class="add-btn" @click="addArtifact">新增产物</button></div><div v-for="(_, index) in currentEnvironmentConfig.artifacts" :key="index" class="artifact-form"><div class="artifact-heading"><strong>产物 {{ index + 1 }}</strong><button class="remove-btn" @click="removeArtifact(index)">删除</button></div><label>构建产物目录<input v-model.trim="currentEnvironmentConfig.artifacts[index].build_output" /></label><label>产物重命名<input v-model.trim="currentEnvironmentConfig.artifacts[index].rename_to" /></label><label>部署脚本<textarea v-model="currentEnvironmentConfig.artifacts[index].deploy_script" rows="5" /></label></div></div>
              </div>
            </template>
            <div v-else class="empty-state">请选择或新增一个子项目</div>
          </div>
          <div v-else class="empty-state">请选择或新增一个项目</div>
        </div>
      </section>
    </template>

    <div v-if="selectedEnvironment" class="dialog-backdrop" @click.self="closeEnvironmentDialog"><form class="environment-dialog" @submit.prevent="saveEnvironment"><div class="section-heading"><h2>{{ environmentTitle }}访问配置</h2><button type="button" class="close-btn" @click="closeEnvironmentDialog">&times;</button></div><label class="switch-row"><span>部署权限</span><input :checked="!environmentDraft.disabled" type="checkbox" @change="toggleEnvironmentDeployPermission" /><span class="switch" aria-hidden="true"></span><span>{{ environmentDraft.disabled ? '关闭' : '开启' }}</span></label><label>用户端地址<input v-model.trim="environmentDraft.links.user_url" type="url" placeholder="http://" /></label><label>管理端地址<input v-model.trim="environmentDraft.links.admin_url" type="url" placeholder="http://" /></label><div class="extras"><div class="section-heading"><strong>其他地址</strong><button type="button" class="add-btn" @click="addExtraLink">新增地址</button></div><div v-for="(_, index) in environmentDraft.links.extra" :key="index" class="extra-row"><input v-model.trim="environmentDraft.links.extra[index].label" placeholder="标签" /><input v-model.trim="environmentDraft.links.extra[index].url" placeholder="URL" /><button type="button" class="remove-btn" @click="removeExtraLink(index)">删除</button></div></div><div class="dialog-actions"><button type="button" class="secondary-btn" @click="closeEnvironmentDialog">取消</button><button class="save-btn" :disabled="savingEnvironment">{{ savingEnvironment ? '保存中...' : '确认保存' }}</button></div></form></div>
    <div v-if="createDialog" class="dialog-backdrop" @click.self="closeCreateDialog"><form class="create-dialog" @submit.prevent="createDialog === 'project' ? addProject() : addSubProject()"><div class="section-heading"><h2>{{ createDialog === 'project' ? '新增项目' : '新增子项目' }}</h2><button type="button" class="close-btn" @click="closeCreateDialog">&times;</button></div><label>{{ createDialog === 'project' ? '项目标识' : '子项目标识' }}<input v-model.trim="newProjectKey" placeholder="请输入标识" autofocus /></label><label>{{ createDialog === 'project' ? '项目名称' : '子项目名称' }}<input v-model.trim="newProjectLabel" placeholder="请输入名称" /></label><div class="dialog-actions"><button type="button" class="secondary-btn" @click="closeCreateDialog">取消</button><button class="save-btn" :disabled="!newProjectKey">确认新增</button></div></form></div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { fetchEditableConfig, updateEnvironmentAccess, updateProjectBuildConfig, type ArtifactConfig, type EditableConfig, type EditableEnvironment, type EnvironmentAccessConfig, type ProjectBuildConfig, type ProjectBuildConfigItem, type SubProjectBuildConfig, type SubProjectEnvironmentConfig } from '../api/index'

const props = defineProps<{ adminToken: string }>()
const standardEnvironments = [{ key: 'dev', label: '开发环境配置' }, { key: 'sit', label: '测试环境配置' }, { key: 'prod', label: '生产环境配置' }]
const environments = ref<EditableConfig['environments']>({})
const projects = ref<ProjectBuildConfig>({})
const serverOptions = ref<string[]>([])
const selectedProjectKey = ref('')
const selectedSubProjectKey = ref('')
const selectedEnvironmentKey = ref('dev')
const newProjectKey = ref('')
const newProjectLabel = ref('')
const projectKeyInput = ref('')
const createDialog = ref<'project' | 'subproject' | null>(null)
const selectedEnvironment = ref<string | null>(null)
const loading = ref(true)
const savingEnvironment = ref(false)
const savingProjects = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const environmentDraft = reactive<EnvironmentAccessConfig>({ disabled: false, links: { user_url: '', admin_url: '', extra: [] } })
const selectedProject = computed<ProjectBuildConfigItem | null>(() => projects.value[selectedProjectKey.value] ?? null)
const selectedSubProject = computed<SubProjectBuildConfig | null>(() => selectedProject.value?.sub_projects[selectedSubProjectKey.value] ?? null)
const currentEnvironmentConfig = computed<SubProjectEnvironmentConfig | null>(() => { const sub = selectedSubProject.value; if (!sub) return null; if (!sub.env_overrides[selectedEnvironmentKey.value]) sub.env_overrides[selectedEnvironmentKey.value] = emptyEnvironmentConfig(); return sub.env_overrides[selectedEnvironmentKey.value] })
const hasArtifacts = computed({ get: () => (currentEnvironmentConfig.value?.artifacts.length ?? 0) > 0, set: value => { const env = currentEnvironmentConfig.value; if (!env) return; env.artifacts = value ? [emptyArtifact()] : [] } })
const environmentTitle = computed(() => standardEnvironments.find(item => item.key === selectedEnvironment.value)?.label.replace('配置', '') ?? '')

function emptyEnvironmentConfig(): SubProjectEnvironmentConfig { return { build_cmd: '', build_output: '', deploy_script: '', rename_to: '', artifacts: [], server: '', disabled: null } }
function emptyArtifact(): ArtifactConfig { return { build_output: '', deploy_script: '', rename_to: '' } }
function selectProject(key: string) { selectedProjectKey.value = key; projectKeyInput.value = key; const keys = Object.keys(projects.value[key].sub_projects); selectedSubProjectKey.value = keys[0] ?? '' }
function selectSubProject(key: string) { selectedSubProjectKey.value = key; ensureEnvironmentConfig() }
function ensureEnvironmentConfig() { void currentEnvironmentConfig.value }
function openCreateDialog(type: 'project' | 'subproject') { newProjectKey.value = ''; newProjectLabel.value = ''; createDialog.value = type }
function closeCreateDialog() { createDialog.value = null }
function addProject() { if (!newProjectKey.value) return; if (projects.value[newProjectKey.value]) { errorMessage.value = '项目标识已存在'; return }; projects.value[newProjectKey.value] = { label: newProjectLabel.value || newProjectKey.value, sub_projects: {} }; selectProject(newProjectKey.value); closeCreateDialog() }
function removeProject() { const project = selectedProject.value; if (!project || !confirm(`是否删除${project.label || selectedProjectKey.value}？`)) return; delete projects.value[selectedProjectKey.value]; selectedProjectKey.value = ''; selectedSubProjectKey.value = '' }
function renameProject() { const oldKey = selectedProjectKey.value; const newKey = projectKeyInput.value; if (!oldKey || newKey === oldKey) return; if (!newKey) { projectKeyInput.value = oldKey; return }; if (projects.value[newKey]) { errorMessage.value = '项目标识已存在'; projectKeyInput.value = oldKey; return }; projects.value[newKey] = projects.value[oldKey]; delete projects.value[oldKey]; selectedProjectKey.value = newKey }
function addSubProject() { const project = selectedProject.value; if (!project || !newProjectKey.value) return; if (project.sub_projects[newProjectKey.value]) { errorMessage.value = '子项目标识已存在'; return }; project.sub_projects[newProjectKey.value] = { label: newProjectLabel.value || newProjectKey.value, build_type: '', env_overrides: { dev: emptyEnvironmentConfig() } }; selectedSubProjectKey.value = newProjectKey.value; closeCreateDialog() }
function removeSubProject() { const project = selectedProject.value; const subProject = selectedSubProject.value; if (!project || !subProject || !confirm(`是否删除${subProject.label || selectedSubProjectKey.value}？`)) return; delete project.sub_projects[selectedSubProjectKey.value]; selectedSubProjectKey.value = Object.keys(project.sub_projects)[0] ?? '' }
function toggleProjectDeployPermission(event: Event) { const env = currentEnvironmentConfig.value; if (env) env.disabled = !(event.target as HTMLInputElement).checked }
function addArtifact() { currentEnvironmentConfig.value?.artifacts.push(emptyArtifact()) }
function removeArtifact(index: number) { currentEnvironmentConfig.value?.artifacts.splice(index, 1) }
function cloneAccess(env: EditableEnvironment): EnvironmentAccessConfig { return { disabled: env.disabled, links: { user_url: env.links?.user_url ?? '', admin_url: env.links?.admin_url ?? '', extra: (env.links?.extra ?? []).map(item => ({ ...item })) } } }
function openEnvironmentDialog(key: string) { const env = environments.value[key]; if (!env) return; Object.assign(environmentDraft, cloneAccess(env)); environmentDraft.links.extra = environmentDraft.links.extra.map(item => ({ ...item })); selectedEnvironment.value = key }
function closeEnvironmentDialog() { if (!savingEnvironment.value) selectedEnvironment.value = null }
function toggleEnvironmentDeployPermission(event: Event) { environmentDraft.disabled = !(event.target as HTMLInputElement).checked }
function addExtraLink() { environmentDraft.links.extra.push({ label: '', url: '' }) }
function removeExtraLink(index: number) { environmentDraft.links.extra.splice(index, 1) }
async function loadConfig() { loading.value = true; try { const config = await fetchEditableConfig(props.adminToken); environments.value = config.environments; projects.value = config.projects; serverOptions.value = config.servers; const first = Object.keys(projects.value)[0]; if (first) selectProject(first) } catch (err: unknown) { errorMessage.value = `获取配置失败：${err instanceof Error ? err.message : '未知错误'}` } finally { loading.value = false } }
async function saveEnvironment() { if (!selectedEnvironment.value || !confirm(`是否确认修改${environmentTitle.value}访问配置？`)) return; savingEnvironment.value = true; try { const saved = await updateEnvironmentAccess(props.adminToken, selectedEnvironment.value, { disabled: environmentDraft.disabled, links: { ...environmentDraft.links, extra: environmentDraft.links.extra.map(item => ({ ...item })) } }); environments.value[selectedEnvironment.value] = { ...environments.value[selectedEnvironment.value], ...saved }; successMessage.value = `${environmentTitle.value}访问配置已保存并生效`; selectedEnvironment.value = null } catch (err: unknown) { errorMessage.value = `保存环境配置失败：${err instanceof Error ? err.message : '未知错误'}` } finally { savingEnvironment.value = false } }
async function saveProjects() { if (!confirm('是否确认修改项目打包配置？')) return; savingProjects.value = true; try { projects.value = await updateProjectBuildConfig(props.adminToken, projects.value); successMessage.value = '项目打包配置已保存并生效' } catch (err: unknown) { errorMessage.value = `保存项目打包配置失败：${err instanceof Error ? err.message : '未知错误'}` } finally { savingProjects.value = false } }
onMounted(loadConfig)
</script>

<style scoped>
.subproject-item.active { border-color:#1d4ed8; background:#2563eb; color:#fff; font-weight:600; box-shadow:0 1px 2px rgb(37 99 235 / 30%); }
.environment-select select { min-height: 38px; padding: 8px 10px; }
.name-fields { display:grid; flex:1; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }
.create-dialog { display:flex; width:min(100%,420px); flex-direction:column; gap:14px; padding:20px; border-radius:8px; background:#fff; box-shadow:0 12px 30px rgb(0 0 0 / 20%); }
.create-dialog>label { display:flex; flex-direction:column; gap:6px; color:#374151; font-size:.88rem; font-weight:600; }
.create-dialog input { box-sizing:border-box; width:100%; padding:8px 10px; border:1px solid #d1d5db; border-radius:6px; font-size:.9rem; }
.config-page { padding: 24px; color: #1a1a1a; } h1,h2,h3 { margin: 0; } h1 { font-size: 1.5rem; } h2 { font-size: 1.1rem; } h3 { font-size: 1rem; } p { margin: 6px 0 0; color: #6b7280; font-size: .92rem; }.page-heading { margin-bottom: 20px; }.config-section { padding: 18px 0 24px; border-top: 1px solid #e5e7eb; }.section-heading,.name-row,.subproject-heading,.artifact-heading { display:flex; align-items:center; justify-content:space-between; gap:12px; }.environment-actions,.subproject-list { display:flex; flex-wrap:wrap; gap:8px; margin-top:16px; }.environment-btn,.project-item,.subproject-item,.save-btn,.secondary-btn,.add-btn,.remove-btn,.inline-add button { border-radius:6px; font-size:.9rem; cursor:pointer; }.environment-btn { padding:9px 14px; border:1px solid #bfdbfe; background:#eff6ff; color:#1d4ed8; font-weight:600; }.environment-btn:disabled { cursor:not-allowed; opacity:.5; }.save-btn { padding:8px 14px; border:1px solid #2563eb; background:#2563eb; color:#fff; font-weight:600; }.save-btn:disabled { cursor:not-allowed; opacity:.6; }.project-layout { display:grid; grid-template-columns:220px minmax(0,1fr); gap:20px; margin-top:16px; }.project-list { display:flex; flex-direction:column; gap:6px; padding-right:16px; border-right:1px solid #e5e7eb; }.project-item,.subproject-item { padding:8px 10px; border:1px solid #d1d5db; background:#fff; color:#374151; text-align:left; }.project-item.active,.subproject-item.active { border-color:#2563eb; background:#eff6ff; color:#1d4ed8; }.inline-add { display:grid; gap:6px; margin-top:10px; }.inline-add input,.inline-add button,.name-row input,.build-form input,.build-form select,.environment-dialog input,.extra-row input { box-sizing:border-box; width:100%; padding:7px 9px; border:1px solid #d1d5db; border-radius:6px; font-size:.88rem; }.inline-add button,.add-btn { padding:7px 10px; border:1px solid #2563eb; background:#fff; color:#2563eb; }.project-editor { min-width:0; }.name-row { margin-bottom:16px; }.name-row label,.environment-select label,.build-form>label,.environment-dialog>label { display:flex; flex-direction:column; gap:6px; color:#374151; font-size:.88rem; font-weight:600; }.remove-btn { padding:6px 10px; border:1px solid #fecaca; background:#fff; color:#dc2626; white-space:nowrap; }.build-form { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; margin-top:18px; }.build-form>label:has(textarea),.artifacts { grid-column:1/-1; }.build-form textarea { min-height:78px; padding:9px; border:1px solid #d1d5db; border-radius:6px; font-family:Consolas,'Courier New',monospace; resize:vertical; }.switch-row { flex-direction:row !important; align-items:center; gap:10px; cursor:pointer; }.switch-row input { position:absolute; opacity:0; }.switch { position:relative; width:38px; height:22px; border-radius:12px; background:#9ca3af; }.switch::after { position:absolute; top:3px; left:3px; width:16px; height:16px; border-radius:50%; background:#fff; content:''; transition:transform .15s; }.switch-row input:checked + .switch { background:#2563eb; }.switch-row input:checked + .switch::after { transform:translateX(16px); }.artifacts { padding-top:8px; }.artifact-form { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; margin-top:10px; padding:12px; border:1px solid #e5e7eb; border-radius:6px; }.artifact-heading,.artifact-form label:last-child { grid-column:1/-1; }.artifact-form label { display:flex; flex-direction:column; gap:6px; font-size:.88rem; }.artifact-form textarea { min-height:90px; padding:8px; border:1px solid #d1d5db; border-radius:6px; font-family:Consolas,'Courier New',monospace; resize:vertical; }.empty-state,.loading-state { padding:32px; text-align:center; color:#6b7280; }.alert { margin-bottom:16px; padding:10px 12px; border:1px solid; border-radius:6px; }.error-alert { border-color:#fca5a5; background:#fef2f2; color:#b91c1c; }.success-alert { border-color:#86efac; background:#f0fdf4; color:#166534; }.dialog-backdrop { position:fixed; z-index:10; inset:0; display:grid; place-items:center; padding:16px; background:rgb(17 24 39 / 45%); }.environment-dialog { display:flex; width:min(100%,640px); max-height:calc(100vh - 32px); flex-direction:column; gap:14px; overflow-y:auto; padding:20px; border-radius:8px; background:#fff; box-shadow:0 12px 30px rgb(0 0 0 / 20%); }.close-btn { border:0; background:transparent; font-size:1.5rem; cursor:pointer; }.extras { margin-top:4px; }.extra-row { display:grid; grid-template-columns:minmax(120px,.7fr) minmax(180px,1.3fr) auto; gap:8px; margin-top:8px; }.dialog-actions { display:flex; justify-content:flex-end; gap:8px; }.secondary-btn { padding:8px 14px; border:1px solid #d1d5db; background:#fff; color:#374151; } @media (max-width:800px) { .project-layout { grid-template-columns:1fr; }.project-list { padding:0 0 16px; border-right:0; border-bottom:1px solid #e5e7eb; }.build-form { grid-template-columns:1fr; }.artifact-form,.extra-row { grid-template-columns:1fr; } }
</style>

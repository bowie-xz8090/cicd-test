<template>
  <div id="app">
    <div class="app-header">
      <h2>{{ siteTitle }}</h2>
    </div>
    <div class="app-layout">
      <div class="panel-left">
        <DeployPage @deployed="onDeployed" />
      </div>
      <div class="panel-right">
        <HistoryPage ref="historyRef" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import DeployPage from './views/DeployPage.vue'
import HistoryPage from './views/HistoryPage.vue'
import { fetchSiteInfo } from './api/index'

const historyRef = ref<InstanceType<typeof HistoryPage> | null>(null)
const siteTitle = ref('自动部署平台')

onMounted(async () => {
  try {
    const info = await fetchSiteInfo()
    siteTitle.value = info.title
    document.title = info.title
  } catch {
    // use default
  }
})

function onDeployed() {
  historyRef.value?.refresh()
}
</script>

<style scoped>
#app {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.app-header {
  padding: 12px 24px;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
}

.app-header h2 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  color: #1a1a1a;
}

.app-layout {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.panel-left {
  width: 30%;
  min-width: 320px;
  border-right: 1px solid #e5e7eb;
  overflow-y: auto;
  padding: 0;
}

.panel-right {
  width: 70%;
  overflow-y: auto;
  padding: 0;
}
</style>

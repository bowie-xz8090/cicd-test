<template>
  <div id="app">
    <nav class="app-nav">
      <router-link to="/" class="nav-link" active-class="nav-active" exact>部署操作</router-link>
      <router-link to="/history" class="nav-link" active-class="nav-active">部署历史</router-link>
    </nav>
    <router-view />
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
#app {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.app-nav {
  display: flex;
  gap: 4px;
  padding: 12px 16px;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
}

.nav-link {
  padding: 6px 16px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 500;
  color: #374151;
  text-decoration: none;
  transition: background 0.15s, color 0.15s;
}

.nav-link:hover {
  background: #e5e7eb;
}

.nav-active {
  background: #2563eb;
  color: #fff;
}

.nav-active:hover {
  background: #1d4ed8;
}
</style>

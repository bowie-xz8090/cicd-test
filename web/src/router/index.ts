import { createRouter, createWebHistory } from 'vue-router'
import { getRuntimeBasePath } from '../utils/basePath'

const router = createRouter({
  history: createWebHistory(getRuntimeBasePath() || '/'),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('../App.vue'),
    },
  ],
})

export default router

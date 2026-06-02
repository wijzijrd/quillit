import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import QuillitView from '../views/QuillitView.vue'
import { useAuthStore } from '../stores/useAuthStore.js'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: DashboardView },
    { path: '/notes', component: QuillitView },
    { path: '/notes/:id', component: QuillitView },
    { path: '/quillit', redirect: '/notes' },
    { path: '/quillit/:id', redirect: to => `/notes/${to.params.id}` },
    // Project-scoped routes — route.params.projectId signals project context
    { path: '/projects/:projectId', component: () => import('../views/ProjectView.vue') },
    { path: '/projects/:projectId/notes', component: QuillitView },
    { path: '/campaigns', component: () => import('../views/CampaignsView.vue') },
    { path: '/players', redirect: '/campaigns' },
    { path: '/member', component: () => import('../views/MemberView.vue') },
    { path: '/share/:token', component: () => import('../views/ShareView.vue'), meta: { public: true } },
    { path: '/tag/:tag', component: () => import('../views/TagView.vue') },
    { path: '/profile', component: () => import('../views/ProfileView.vue') },
    { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/register', component: () => import('../views/SetupView.vue'), meta: { public: true } },
    { path: '/admin', component: () => import('../views/AdminView.vue'), meta: { adminOnly: true } },
    { path: '/admin/categories', redirect: '/admin' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true

  const auth = useAuthStore()
  await auth.fetchMe()
  if (!auth.isLoggedIn) {
    // Preserve the original destination so LoginView can redirect back after auth.
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  if (to.meta.adminOnly && auth.user?.role !== 'admin') return '/'
})

export default router

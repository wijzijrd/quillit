import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import QuillitView from '../views/QuillitView.vue'
import { useAuthStore } from '../stores/useAuthStore'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: DashboardView },
    { path: '/entries', component: QuillitView },
    { path: '/entries/:id', component: QuillitView },
    { path: '/notes', redirect: '/entries' },
    { path: '/notes/:id', redirect: (to) => `/entries/${to.params.id}` },
    { path: '/quillit', redirect: '/entries' },
    { path: '/quillit/:id', redirect: (to) => `/entries/${to.params.id}` },
    { path: '/projects/:projectId', component: () => import('../views/ProjectView.vue') },
    { path: '/projects/:projectId/game', component: () => import('../views/GameView.vue') },
    { path: '/projects/:projectId/entries', component: QuillitView },
    { path: '/projects/:projectId/entries/:id', component: QuillitView },
    { path: '/projects/:projectId/notes', redirect: (to) => `/projects/${to.params.projectId}/entries` },
    { path: '/tag/:tag', component: () => import('../views/TagView.vue') },
    { path: '/profile', component: () => import('../views/ProfileView.vue') },
    { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/register', component: () => import('../views/SetupView.vue'), meta: { public: true } },
    { path: '/forgot-password', component: () => import('../views/ForgotPasswordView.vue'), meta: { public: true } },
    { path: '/reset-password', component: () => import('../views/ResetPasswordView.vue'), meta: { public: true } },
    { path: '/admin', component: () => import('../views/AdminView.vue'), meta: { adminOnly: true } },
    { path: '/admin/categories', redirect: '/admin' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const auth = useAuthStore()
  await auth.fetchMe()
  if (!auth.isLoggedIn) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.adminOnly && auth.user?.role !== 'admin') return '/'
})

export default router

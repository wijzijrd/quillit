import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router/index'
import App from './App.vue'
import { useUIStore } from './stores/useUIStore'
import './assets/main.css'

const app = createApp(App)
app.use(createPinia())
// Apply season/glass before mount so dark seasons don't flash light.
useUIStore().initTheme()
app.use(router)
await router.isReady()
app.mount('#app')

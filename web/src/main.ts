import { createApp } from 'vue'

import App from './App.vue'
import { registerApplicationServiceWorker } from './pwa'
import './style.css'

createApp(App).mount('#app')
registerApplicationServiceWorker()

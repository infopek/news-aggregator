import { createApp } from 'vue'

import App from './App.vue'
import { registerApplicationServiceWorker } from './pwa'
import './styles/app.css'

createApp(App).mount('#app')
registerApplicationServiceWorker()

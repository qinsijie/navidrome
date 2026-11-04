window.global = window // fix "global is not defined" error in react-image-lightbox

import config from './config'
import { activateVConsole } from './utils/vconsole'

// Mount vConsole only in Debug mode when ?debug=1 or localStorage.nd_debug === '1'
if (
  config.isDebug &&
  typeof window !== 'undefined' &&
  (window.location.search.includes('debug=1') ||
    localStorage.getItem('nd_debug') === '1')
) {
  activateVConsole()
}

import ReactDOM from 'react-dom'
import './index.css'
import App from './App'
import { registerSW } from 'virtual:pwa-register'

registerSW({ immediate: true })

ReactDOM.render(<App />, document.getElementById('root'))

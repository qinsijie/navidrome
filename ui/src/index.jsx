window.global = window // fix "global is not defined" error in react-image-lightbox

// Mount vConsole if ?debug=1 or localStorage.nd_debug === '1'
if (
  typeof window !== 'undefined' &&
  (window.location.search.includes('debug=1') ||
    localStorage.getItem('nd_debug') === '1')
) {
  const script = document.createElement('script')
  script.src = '/vconsole.min.js'
  script.onload = () => {
    if (window.VConsole) {
      new window.VConsole()
      // eslint-disable-next-line no-console
      console.log('[vConsole] Activated in debug mode')
    }
  }
  document.head.appendChild(script)
}

import ReactDOM from 'react-dom'
import './index.css'
import App from './App'
import { registerSW } from 'virtual:pwa-register'

registerSW({ immediate: true })

ReactDOM.render(<App />, document.getElementById('root'))

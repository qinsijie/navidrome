import config from '../config'

export const activateVConsole = () => {
  if (!config.isDebug) {
    return
  }
  try {
    localStorage.setItem('nd_debug', '1')
  } catch {
    // ignore storage quota / access error
  }
  if (typeof window !== 'undefined') {
    if (window.vConsoleInstance) {
      if (typeof window.vConsoleInstance.show === 'function') {
        window.vConsoleInstance.show()
      }
      return
    }
    if (window.VConsole) {
      window.vConsoleInstance = new window.VConsole()
    } else {
      const script = document.createElement('script')
      script.src = '/vconsole.min.js'
      script.onload = () => {
        if (window.VConsole && !window.vConsoleInstance) {
          window.vConsoleInstance = new window.VConsole()
          // eslint-disable-next-line no-console
          console.log('[vConsole] Activated in debug mode')
        }
      }
      document.head.appendChild(script)
    }
  }
}

export const useEasterEggVConsole = () => {
  return () => {
    if (!config.isDebug) return
    const now = Date.now()
    if (!window.__nd_tap_last || now - window.__nd_tap_last > 3500) {
      window.__nd_tap_count = 1
    } else {
      window.__nd_tap_count = (window.__nd_tap_count || 0) + 1
    }
    window.__nd_tap_last = now

    if (window.__nd_tap_count >= 4) {
      window.__nd_tap_count = 0
      activateVConsole()
    }
  }
}

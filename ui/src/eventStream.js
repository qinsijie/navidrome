import { baseUrl } from './utils'
import throttle from 'lodash.throttle'
import { processEvent, serverDown, streamReconnected } from './actions'
import { REST_URL } from './consts'
import config from './config'
import { clientUniqueId } from './dataProvider'

const newEventStream = async () => {
  let url = baseUrl(`${REST_URL}/events`)
  const params = new URLSearchParams()
  if (localStorage.getItem('token')) {
    params.append('jwt', localStorage.getItem('token'))
  }
  if (clientUniqueId) {
    params.append('clientUniqueId', clientUniqueId)
  }
  const queryStr = params.toString()
  if (queryStr) {
    url = url + `?${queryStr}`
  }
  return new EventSource(url)
}

let eventStream
let reconnectTimer
let reconnectDelay = 1000
let consecutiveErrors = 0

const setupHandlers = (stream, dispatchFn) => {
  stream.onopen = () => {
    // eslint-disable-next-line no-console
    console.log('EventStream connected')
    reconnectDelay = 1000
    consecutiveErrors = 0
    dispatchFn(streamReconnected())
  }

  stream.addEventListener('serverStart', eventHandler(dispatchFn))
  stream.addEventListener('scanStatus', throttledEventHandler(dispatchFn))
  stream.addEventListener('refreshResource', eventHandler(dispatchFn))
  stream.addEventListener('userPlaybackState', (event) => {
    try {
      const data = JSON.parse(event.data)
      window.dispatchEvent(
        new CustomEvent('nd:user-playback-state', { detail: data }),
      )
    } catch (e) {}
  })
  if (config.enableNowPlaying) {
    stream.addEventListener('nowPlayingCount', eventHandler(dispatchFn))
  }
  stream.addEventListener('keepAlive', eventHandler(dispatchFn))
  stream.onerror = (e) => {
    consecutiveErrors++
    if (consecutiveErrors <= 2) {
      // eslint-disable-next-line no-console
      console.log('EventStream connection issue, reconnecting...', e)
    }
    // Only mark server as down if connection failed multiple times consecutively
    if (consecutiveErrors >= 3) {
      dispatchFn(serverDown())
    }
    if (stream) {
      stream.close()
    }
    scheduleReconnect(dispatchFn)
  }
}

const scheduleReconnect = (dispatchFn) => {
  if (!reconnectTimer) {
    const delay = reconnectDelay
    reconnectDelay = Math.min(10000, Math.floor(reconnectDelay * 1.5))
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect(dispatchFn)
    }, delay)
  }
}

const connect = async (dispatchFn) => {
  try {
    if (eventStream) {
      eventStream.close()
      eventStream = null
    }
    const stream = await newEventStream()
    eventStream = stream
    setupHandlers(stream, dispatchFn)
    return stream
  } catch (e) {
    // eslint-disable-next-line no-console
    console.log(`Error connecting to EventStream:`, e)
    scheduleReconnect(dispatchFn)
  }
}

const eventHandler = (dispatchFn) => (event) => {
  const data = JSON.parse(event.data)
  if (event.type !== 'keepAlive') {
    dispatchFn(processEvent(event.type, data))
  }
}

const throttledEventHandler = (dispatchFn) =>
  throttle(eventHandler(dispatchFn), 100, { trailing: true })

const startEventStreamLegacy = async (dispatchFn) => {
  return newEventStream()
    .then((newStream) => {
      newStream.addEventListener('serverStart', eventHandler(dispatchFn))
      newStream.addEventListener(
        'scanStatus',
        throttledEventHandler(dispatchFn),
      )
      newStream.addEventListener('refreshResource', eventHandler(dispatchFn))
      newStream.addEventListener('userPlaybackState', (event) => {
        try {
          const data = JSON.parse(event.data)
          window.dispatchEvent(
            new CustomEvent('nd:user-playback-state', { detail: data }),
          )
        } catch (e) {}
      })
      if (config.enableNowPlaying) {
        newStream.addEventListener('nowPlayingCount', eventHandler(dispatchFn))
      }
      newStream.addEventListener('keepAlive', eventHandler(dispatchFn))
      newStream.onerror = (e) => {
        // eslint-disable-next-line no-console
        console.log('EventStream error', e)
        dispatchFn(serverDown())
      }
      return newStream
    })
    .catch((e) => {
      // eslint-disable-next-line no-console
      console.log(`Error connecting to server:`, e)
    })
}

const startEventStreamNew = async (dispatchFn) => {
  if (eventStream) {
    eventStream.close()
    eventStream = null
  }
  return connect(dispatchFn)
}

let recoveryListenersSetup = false
const setupRecoveryListeners = (dispatchFn) => {
  if (recoveryListenersSetup || typeof document === 'undefined') return
  recoveryListenersSetup = true

  const handleWakeup = () => {
    if (
      document.visibilityState === 'visible' &&
      localStorage.getItem('is-authenticated')
    ) {
      if (!eventStream || eventStream.readyState === 2 /* CLOSED */) {
        // eslint-disable-next-line no-console
        console.log('App resumed: reconnecting EventStream')
        startEventStreamNew(dispatchFn)
      }
    }
  }

  document.addEventListener('visibilitychange', handleWakeup)
  window.addEventListener('online', handleWakeup)
}

const startEventStream = async (dispatchFn) => {
  if (!localStorage.getItem('is-authenticated')) {
    return Promise.resolve()
  }
  setupRecoveryListeners(dispatchFn)
  return startEventStreamNew(dispatchFn)
}

export { startEventStream }

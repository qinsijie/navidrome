import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom'
import { useInterval } from '../common'
import { useDispatch, useSelector } from 'react-redux'
import { useMediaQuery } from '@material-ui/core'
import { ThemeProvider } from '@material-ui/core/styles'
import {
  createMuiTheme,
  useAuthState,
  useDataProvider,
  useNotify,
  useTranslate,
} from 'react-admin'
import ReactGA from 'react-ga'
import { GlobalHotKeys } from 'react-hotkeys'
import ReactJkMusicPlayer from 'navidrome-music-player'
import 'navidrome-music-player/assets/index.css'
import useCurrentTheme from '../themes/useCurrentTheme'
import config from '../config'
import useStyle from './styles'
import AudioTitle from './AudioTitle'
import {
  clearQueue,
  currentPlaying,
  refreshQueue,
  setPlayMode,
  setTranscodingProfile,
  setVolume,
  syncQueue,
} from '../actions'
import PlayerToolbar from './PlayerToolbar'
import { sendNotification, sanitizeImageUrl } from '../utils'
import subsonic from '../subsonic'
import locale from './locale'
import { keyMap } from '../hotkeys'
import keyHandlers from './keyHandlers'
import { calculateGain } from '../utils/calculateReplayGain'
import {
  detectBrowserProfile,
  decisionService,
  DIRECT_STREAM_FORMATS,
} from '../transcode'
import { httpClient } from '../dataProvider'

class PlayerErrorBoundary extends React.Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, errorInfo) {
    console.error('[PlayerErrorBoundary] Captured player error:', error, errorInfo)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div
          style={{
            position: 'fixed',
            bottom: 0,
            left: 0,
            right: 0,
            background: 'rgba(50, 10, 10, 0.95)',
            color: '#ff8888',
            padding: '8px 16px',
            zIndex: 99999,
            fontSize: '12px',
            fontFamily: 'monospace',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
          }}
        >
          <span>播放器渲染受阻: {String(this.state.error?.message || this.state.error)}</span>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            style={{
              background: '#552222',
              color: '#fff',
              border: 'none',
              padding: '4px 8px',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            重试
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

const SILENT_AUDIO_URI =
  'data:audio/wav;base64,UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA='

const logAudioGuard = (msg, ...args) => {
  const d = new Date()
  const timeStr = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}.${String(d.getMilliseconds()).padStart(3, '0')}`
  // eslint-disable-next-line no-console
  console.log(`[${timeStr}] [AudioGuard] ${msg}`, ...args)
}

const warnAudioGuard = (msg, ...args) => {
  const d = new Date()
  const timeStr = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}.${String(d.getMilliseconds()).padStart(3, '0')}`
  // eslint-disable-next-line no-console
  console.warn(`[${timeStr}] [AudioGuard] ${msg}`, ...args)
}

const Player = () => {
  const theme = useCurrentTheme()
  const translate = useTranslate()
  const notify = useNotify()
  const playerTheme = theme.player?.theme || 'dark'
  const dataProvider = useDataProvider()
  const playerState = useSelector((state) => state.player)
  const dispatch = useDispatch()
  const { authenticated } = useAuthState()
  const isDesktop = useMediaQuery('(min-width:810px)')
  const isMobilePlayer =
    /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
      navigator.userAgent,
    )

  // 1. All State declarations at the very top
  const [currentTrackId, setCurrentTrackId] = useState(null)
  const [heartbeatTrackId, setHeartbeatTrackId] = useState(null)
  const [audioInstance, setAudioInstance] = useState(null)
  const [activeOutputDevice, setActiveOutputDevice] = useState('local')
  const [context, setContext] = useState(null)
  const [gainNode, setGainNode] = useState(null)

  // 2. All Ref declarations at the very top (strictly before any effect/interval/callback to prevent TDZ)
  const lastPositionMsRef = useRef(0)
  const currentTrackIdRef = useRef(null)
  const stoppedRef = useRef(false)
  const isUserPausedRef = useRef(false)
  const audioInstanceRef = useRef(null)
  audioInstanceRef.current = audioInstance
  const playerStateRef = useRef(playerState)
  playerStateRef.current = playerState
  const activeOutputDeviceRef = useRef(activeOutputDevice)
  activeOutputDeviceRef.current = activeOutputDevice
  const activeDeviceNameRef = useRef('HQPlayer')
  const isQueueOpenRef = useRef(false)
  const isHiddenRef = useRef(false)
  const isSwitchingTargetRef = useRef(false)
  const saveTerminalQueueTimerRef = useRef(null)
  const virtualSecRef = useRef(0)
  const remoteProgressRef = useRef({
    baseSec: 0,
    baseClientTime: 0,
    playing: false,
    duration: 0,
    trackId: null,
    currentIndex: -1,
  })
  const lastSyncedQueueKeyRef = useRef('')
  const remoteSyncLockUntilRef = useRef(0)
  const lastSyncedRemotePlayingRef = useRef(null)
  const isSyntheticEventRef = useRef(false)
  const localQueueSnapshotRef = useRef(null)
  const isUserExplicitlyPausedRef = useRef(false)
  const silentKeepAliveRef = useRef(null)
  const interruptionTimerRef = useRef(null)
  const needsNetworkResumeRef = useRef(false)
  const pendingSeekTargetRef = useRef(null)
  const lastWatchdogTimeRef = useRef(0)
  const watchdogStallCountRef = useRef(0)
  const lastUserInteractionTimeRef = useRef(0)
  const lastReanchorTimeRef = useRef(0)
  const trackErrorCountRef = useRef({})
  const consecutiveSkipCountRef = useRef(0)

  // Global user interaction tracker & slider touchcancel / blur cleanup guard
  useEffect(() => {
    const recordUserAction = () => {
      lastUserInteractionTimeRef.current = Date.now()
    }
    const handleSliderTouchStart = (e) => {
      lastUserInteractionTimeRef.current = Date.now()
      if (
        e.target &&
        (e.target.classList?.contains('rc-slider') ||
          e.target.closest?.('.rc-slider'))
      ) {
        logAudioGuard('[Slider:touch] Touch start on progress slider')
      }
    }
    const handleGlobalTouchEndOrCancel = (e) => {
      if (e.type === 'touchcancel') {
        warnAudioGuard(
          '[Slider:touch] touchcancel event detected! Forcing release of dangling slider drag',
        )
        try {
          document.dispatchEvent(new Event('touchend'))
        } catch (err) {}
      } else if (e.type === 'blur') {
        logAudioGuard(
          '[Window:blur] Window blur detected, dispatching synthetic touchend & mouseup',
        )
        try {
          document.dispatchEvent(new Event('touchend'))
          document.dispatchEvent(new Event('mouseup'))
        } catch (err) {}
      }
    }

    window.addEventListener('pointerdown', recordUserAction, { passive: true })
    window.addEventListener('keydown', recordUserAction, { passive: true })
    window.addEventListener('touchstart', handleSliderTouchStart, { passive: true })
    window.addEventListener('touchcancel', handleGlobalTouchEndOrCancel, {
      passive: true,
    })
    window.addEventListener('blur', handleGlobalTouchEndOrCancel, { passive: true })

    return () => {
      window.removeEventListener('pointerdown', recordUserAction)
      window.removeEventListener('keydown', recordUserAction)
      window.removeEventListener('touchstart', handleSliderTouchStart)
      window.removeEventListener('touchcancel', handleGlobalTouchEndOrCancel)
      window.removeEventListener('blur', handleGlobalTouchEndOrCancel)
    }
  }, [])

  const startSilentKeepAlive = useCallback(() => {
    try {
      if (!silentKeepAliveRef.current) {
        const AudioCtx = window.AudioContext || window.webkitAudioContext
        if (AudioCtx) {
          const ctx = new AudioCtx()
          const osc = ctx.createOscillator()
          const gain = ctx.createGain()
          gain.gain.value = 0.00001
          osc.connect(gain)
          gain.connect(ctx.destination)
          osc.start()
          silentKeepAliveRef.current = { ctx, osc, gain }
        }
      }
      if (
        silentKeepAliveRef.current?.ctx &&
        silentKeepAliveRef.current.ctx.state === 'suspended'
      ) {
        silentKeepAliveRef.current.ctx.resume().catch(() => {})
      }
    } catch (e) {}
  }, [])

  const stopSilentKeepAlive = useCallback(() => {
    try {
      if (
        silentKeepAliveRef.current?.ctx &&
        silentKeepAliveRef.current.ctx.state === 'running'
      ) {
        silentKeepAliveRef.current.ctx.suspend().catch(() => {})
      }
    } catch (e) {}
  }, [])

  const reanchorAudioStream = useCallback((audio, targetSec) => {
    if (!audio) return
    const now = Date.now()
    if (
      now - lastReanchorTimeRef.current < 8000 &&
      lastReanchorTimeRef.current > 0
    ) {
      logAudioGuard('[StreamReanchor] Throttled by 8s cooldown, skipping.')
      return
    }
    lastReanchorTimeRef.current = now

    try {
      const curSrc = audio.src
      if (curSrc && !curSrc.startsWith('blob:') && !curSrc.startsWith('data:')) {
        const u = new URL(curSrc, window.location.href)
        u.searchParams.set('_t', String(now))
        logAudioGuard(
          `[StreamReanchor] Re-anchoring stream with _t=${u.searchParams.get('_t')}, targetSec=${targetSec?.toFixed(2)}s`,
        )
        pendingSeekTargetRef.current = targetSec
        audio.src = u.toString()
        audio.load()
        const p = audio.play()
        if (p && p.then) {
          p.then(() => {
            logAudioGuard('[StreamReanchor] Re-anchored stream play succeeded')
          }).catch((err) => {
            warnAudioGuard(
              '[StreamReanchor] Re-anchored stream play rejected:',
              err,
            )
            // If play rejected in background (DOMException / NotAllowedError), do NOT re-anchor in a loop!
            needsNetworkResumeRef.current = true
          })
        }
      }
    } catch (e) {
      warnAudioGuard('[StreamReanchor] Re-anchor failed with exception:', e)
    }
  }, [])

  // Watchdog: Check every 3 seconds for fake playing (time stalled for >= 6s with buffer starvation readyState <= 2)
  useInterval(
    () => {
      if (
        activeOutputDeviceRef.current !== 'local' ||
        isUserExplicitlyPausedRef.current ||
        !audioInstance ||
        audioInstance.paused ||
        audioInstance.ended ||
        !currentTrackIdRef.current
      ) {
        watchdogStallCountRef.current = 0
        lastWatchdogTimeRef.current = audioInstance?.currentTime || 0
        return
      }

      const curTime = audioInstance.currentTime || 0
      const prevTime = lastWatchdogTimeRef.current
      const readyState = audioInstance.readyState
      const networkState = audioInstance.networkState

      if (Math.abs(curTime - prevTime) < 0.2) {
        watchdogStallCountRef.current += 1
        logAudioGuard(
          `[Watchdog] Stagnant time detected (${watchdogStallCountRef.current}/3): curTime=${curTime.toFixed(2)}s, readyState=${readyState}, networkState=${networkState}, onLine=${navigator.onLine}`,
        )

        // Require 3 consecutive stagnant ticks (>= 6s freeze)
        if (watchdogStallCountRef.current >= 3) {
          watchdogStallCountRef.current = 0

          // STRICT RULE 1: If buffer is充足 (readyState >= 3: HAVE_FUTURE_DATA / HAVE_ENOUGH_DATA),
          // media decoder has data and is not starved. DO NOT re-anchor stream!
          if (readyState >= 3) {
            logAudioGuard(
              `[Watchdog] Buffer is sufficient (readyState=${readyState} >= 3). Skipping stream reload to prevent false interruption.`,
            )
            return
          }

          // STRICT RULE 2: If offline, suspend gracefully and wait for online
          if (!navigator.onLine) {
            logAudioGuard(
              '[Watchdog] Network is offline, marking needsNetworkResumeRef and suspending until network returns',
            )
            needsNetworkResumeRef.current = true
            lastPositionMsRef.current = Math.floor(curTime * 1000)
            return
          }

          // STRICT RULE 3: Cooldown guard (at least 8s between reanchors)
          const now = Date.now()
          if (
            now - lastReanchorTimeRef.current < 8000 &&
            lastReanchorTimeRef.current > 0
          ) {
            logAudioGuard(
              `[Watchdog] Reanchor cooldown active (${Math.round((8000 - (now - lastReanchorTimeRef.current)) / 1000)}s remaining), skipping.`,
            )
            return
          }

          warnAudioGuard(
            `[Watchdog] Audio buffer starved (readyState=${readyState} <= 2) and time frozen for >= 6s! Triggering reanchor at ${curTime.toFixed(2)}s...`,
          )
          reanchorAudioStream(audioInstance, curTime)
        }
      } else {
        watchdogStallCountRef.current = 0
      }
      lastWatchdogTimeRef.current = curTime
    },
    activeOutputDeviceRef.current === 'local' &&
      !isUserExplicitlyPausedRef.current
      ? 3000
      : null,
  )

  const recordRecentlyPlayed = useCallback((info) => {
    if (!info || info.isPlaceholder) return
    const trackId = info.trackId || info.id
    if (!trackId) return

    try {
      const STORAGE_KEY = 'nd_recently_played_records'
      const raw = localStorage.getItem(STORAGE_KEY)
      let list = []
      if (raw) {
        try {
          list = JSON.parse(raw)
        } catch (e) {
          list = []
        }
      }
      if (!Array.isArray(list)) list = []

      list = list.filter((t) => t.id !== trackId)

      const song = info.song || {}
      const duration = info.duration || song.duration || 0
      const title = song.title || info.name || ''
      const artist =
        song.artist ||
        (typeof info.singer === 'string'
          ? info.singer.split(' · ')[0]
          : '') ||
        '未知歌手'
      const album = song.album || ''
      const cover = sanitizeImageUrl(
        info.cover ||
          song.cover ||
          (trackId ? subsonic.getCoverArtUrl({ id: trackId }, 300) : ''),
      )
      const path = song.path || info.musicSrc || ''

      let source = 'local'
      let songId = ''
      let quality = 'flac'
      if (path.startsWith('online://')) {
        const parts = path.replace('online://', '').split('/')
        source = parts[0] || 'online'
        songId = parts[1] || ''
        quality = parts[2] || 'flac'
      } else if (trackId.startsWith('online:')) {
        const parts = trackId.replace('online:', '').split(':')
        source = parts[0] || 'online'
        songId = parts[1] || ''
      }

      const item = {
        id: trackId,
        songId,
        title,
        artist,
        album,
        duration,
        cover,
        path,
        source,
        quality,
        playedAt: Date.now(),
      }

      list.unshift(item)
      if (list.length > 200) {
        list = list.slice(0, 200)
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(list))
      window.dispatchEvent(
        new CustomEvent('nd:recently-played-updated', { detail: item }),
      )
    } catch (e) {}
  }, [])

  currentTrackIdRef.current = currentTrackId

  useInterval(
    () => {
      if (
        activeOutputDeviceRef.current === 'local' &&
        heartbeatTrackId &&
        !stoppedRef.current
      ) {
        subsonic
          .reportPlayback(
            heartbeatTrackId,
            lastPositionMsRef.current,
            'playing',
          )
          .catch(() => {})
      }
    },
    heartbeatTrackId && activeOutputDeviceRef.current === 'local'
      ? config.playbackReportIntervalMs
      : null,
  )

  // Detect browser codec profile and eagerly resolve transcode URLs for the
  // persisted queue once on mount (e.g. after a browser refresh)
  useEffect(() => {
    const profile = detectBrowserProfile()
    decisionService.setProfile(profile)
    dispatch(setTranscodingProfile(profile))

    const state = playerStateRef.current
    const currentIdx = state.savedPlayIndex || 0
    const itemsToResolve = state.queue
      .slice(currentIdx, currentIdx + 4)
      .filter(
        (item) =>
          !item.isRadio &&
          item.trackId &&
          !item.trackId.startsWith('online:') &&
          !(
            item.song?.suffix &&
            DIRECT_STREAM_FORMATS.has(String(item.song.suffix).toLowerCase())
          ),
      )

    if (itemsToResolve.length === 0) {
      if (activeOutputDeviceRef.current === 'local' && (!state.queue || state.queue.length === 0)) {
        httpClient('/api/player/terminal-queue')
          .then(({ json }) => {
            if (json && json.found && json.queue && json.queue.length > 0) {
              const idx =
                json.currentIndex >= 0 && json.currentIndex < json.queue.length
                  ? json.currentIndex
                  : 0
              const restoredQueue = json.queue.map((t) => ({
                trackId: t.trackId,
                uuid: t.trackId,
                name: t.title,
                singer: t.artist,
                duration: t.duration,
                cover: sanitizeImageUrl(
                  t.cover || t.coverUrl || subsonic.getCoverArtUrl({ id: t.trackId }, 300),
                ),
                musicSrc: subsonic.streamUrl(t.trackId) || SILENT_AUDIO_URI,
                song: {
                  id: t.trackId,
                  title: t.title,
                  artist: t.artist,
                  album: t.album,
                  duration: t.duration,
                },
              }))
              dispatch(syncQueue(restoredQueue[idx], restoredQueue))
              if (json.timeOffset > 0) {
                lastPositionMsRef.current = Math.floor(json.timeOffset * 1000)
                if (restoredQueue[idx]?.trackId) {
                  try {
                    sessionStorage.setItem(
                      'nd_pos_' + restoredQueue[idx].trackId,
                      String(json.timeOffset),
                    )
                  } catch (e) {}
                }
              }
            }
          })
          .catch(() => {})
      }
      dispatch(refreshQueue())
      return
    }

    Promise.allSettled(
      itemsToResolve.map((item) =>
        decisionService
          .resolveStreamUrl(item.trackId, item.song?.suffix)
          .then((url) => [item.trackId, url]),
      ),
    ).then((results) => {
      const resolvedUrls = {}
      results.forEach((r) => {
        if (r.status === 'fulfilled') {
          resolvedUrls[r.value[0]] = r.value[1]
        }
      })
      dispatch(refreshQueue(resolvedUrls))
    })
  }, [dispatch])

  // Pre-fetch transcode decisions for next 2-3 songs when queue or position changes
  useEffect(() => {
    if (!playerState.queue.length) return

    const currentIdx = playerState.savedPlayIndex || 0
    const nextSongIds = playerState.queue
      .slice(currentIdx + 1, currentIdx + 4)
      .filter(
        (item) =>
          !item.isRadio &&
          item.trackId &&
          !item.trackId.startsWith('online:') &&
          !(
            item.song?.suffix &&
            DIRECT_STREAM_FORMATS.has(String(item.song.suffix).toLowerCase())
          ),
      )
      .map((item) => item.trackId)

    if (nextSongIds.length > 0) {
      decisionService.prefetchDecisions(nextSongIds)
    }
  }, [playerState.queue, playerState.savedPlayIndex])

  const syncChannelQueueToRedux = useCallback(
    (channelData, force = false) => {
      if (!channelData) return
      const queue = channelData.queue || []
      if (!Array.isArray(queue) || queue.length === 0) return

      const currentQueue = playerStateRef.current?.queue || []
      const serverQueueKey = queue.map((t) => t.trackId).join(',')
      const localQueueKey = currentQueue.map((t) => t.trackId).join(',')
      if (serverQueueKey && (serverQueueKey !== localQueueKey || force)) {
        const mappedList = queue.map((t) => ({
          trackId: t.trackId,
          uuid: t.trackId,
          name: t.title,
          song: {
            id: t.trackId,
            title: t.title,
            artist: t.artist,
            album: t.album,
            duration: t.duration,
          },
          singer: t.artist,
          duration: t.duration,
          musicSrc: subsonic.streamUrl(t.trackId) || SILENT_AUDIO_URI,
          cover: sanitizeImageUrl(
            t.cover || t.coverUrl || subsonic.getCoverArtUrl({ id: t.trackId }, 300),
          ),
        }))
        const targetIdx =
          channelData.currentIndex >= 0 &&
          channelData.currentIndex < mappedList.length
            ? channelData.currentIndex
            : 0
        dispatch(syncQueue(mappedList[targetIdx], mappedList))
        dispatch(currentPlaying(mappedList[targetIdx]))
      }
    },
    [dispatch],
  )

  // Listen to server UserSession SSE events for multi-device synchronized playback (Remote mode only)
  useEffect(() => {
    const handleUserPlaybackState = (e) => {
      const serverState = e.detail
      if (!serverState) return

      // Scheme A: Local mode is 100% client-autonomous. Discard all server playback broadcasts.
      const isViewingRemote = activeOutputDeviceRef.current !== 'local'
      if (!isViewingRemote) return

      const eventChannel =
        serverState.channel ||
        serverState.activeEvent ||
        (serverState.outputTarget && serverState.outputTarget !== 'local'
          ? 'remote'
          : 'local')

      // Current client is acting as Remote Controller (e.g. for HQPlayer)
      // ONLY accept remote channel updates; ignore local audio updates
      if (eventChannel !== 'remote') return

      const remoteData = serverState.remote || serverState

      // 1. Local HTML5 audio element must be silent/paused
      if (audioInstance && !audioInstance.paused) {
        isSyntheticEventRef.current = true
        audioInstance.pause()
        isSyntheticEventRef.current = false
      }

      // 2. Synchronize remote queue if changed
      if (
        remoteData.queue &&
        Array.isArray(remoteData.queue) &&
        remoteData.queue.length > 0
      ) {
        syncChannelQueueToRedux(remoteData)
      }

      // 3. Synchronize remote progress clock
      remoteProgressRef.current.baseSec = remoteData.timeOffset || 0
      remoteProgressRef.current.baseClientTime =
        remoteData.startedAt > 0 ? remoteData.startedAt : Date.now()
      remoteProgressRef.current.playing = remoteData.state === 'playing'
      remoteProgressRef.current.duration =
        remoteData.duration || remoteData.track?.duration || 0
      remoteProgressRef.current.trackId =
        remoteData.trackId || remoteData.track?.trackId || null
      remoteProgressRef.current.currentIndex =
        remoteData.currentIndex ?? -1

      // 4. Sync Redux current track so player UI bar updates
      const trackId = remoteProgressRef.current.trackId
      if (trackId && playerStateRef.current?.current?.trackId !== trackId) {
        const track = remoteData.track || remoteData
        const currentObj = {
          trackId,
          uuid: trackId,
          name: track.title,
          singer: track.artist,
          duration: track.duration,
          cover: sanitizeImageUrl(
            track.cover || track.coverUrl || subsonic.getCoverArtUrl({ id: trackId }, 300),
          ),
          song: {
            id: trackId,
            title: track.title,
            artist: track.artist,
            album: track.album,
            duration: track.duration,
          },
        }
        dispatch(currentPlaying(currentObj))
        currentTrackIdRef.current = trackId
        setCurrentTrackId(trackId)
      }

      // 5. Update play/pause icon in UI without triggering onAudioPlay / onAudioPause network storm
      if (audioInstance) {
        isSyntheticEventRef.current = true
        if (remoteData.state === 'playing') {
          audioInstance.dispatchEvent(new Event('play'))
        } else {
          audioInstance.dispatchEvent(new Event('pause'))
        }
        isSyntheticEventRef.current = false
      }
      lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
    }

    window.addEventListener('nd:user-playback-state', handleUserPlaybackState)
    return () => {
      window.removeEventListener('nd:user-playback-state', handleUserPlaybackState)
    }
  }, [audioInstance, syncChannelQueueToRedux, dispatch])

  // Listen to popstate so mobile back gesture closes queue panel without leaving page
  useEffect(() => {
    const handlePopState = () => {
      if (isQueueOpenRef.current) {
        const closeBtn = document.querySelector(
          '.audio-lists-panel-header-close-btn',
        )
        if (closeBtn) {
          isQueueOpenRef.current = false
          closeBtn.click()
        }
      }
    }
    window.addEventListener('popstate', handlePopState)
    return () => {
      window.removeEventListener('popstate', handlePopState)
    }
  }, [])

  useEffect(() => {
    httpClient('/api/output/devices?enabled_only=true')
      .then(({ json }) => {
        if (json?.devices) {
          const activeDev = json.devices.find((d) => d.id === 'hqplayer')
          if (activeDev?.name) {
            activeDeviceNameRef.current = activeDev.name
          }
        }
      })
      .catch(() => {})

    const handleDeviceChanged = (e) => {
      const newId = e.detail?.deviceId || 'local'
      const prevId = activeOutputDeviceRef.current
      if (prevId === newId) return

      setActiveOutputDevice(newId)
      activeOutputDeviceRef.current = newId
      if (e.detail?.device?.name) {
        activeDeviceNameRef.current = e.detail.device.name
      }
      const isNowRemote = newId !== 'local'
      if (isNowRemote) {
        isSwitchingTargetRef.current = true
        // 1. Pause and mute local audio instance
        if (audioInstanceRef.current) {
          isSyntheticEventRef.current = true
          audioInstanceRef.current.pause()
          audioInstanceRef.current.muted = true
          isSyntheticEventRef.current = false
        }

        // Save local queue snapshot before switching to remote view
        localQueueSnapshotRef.current = {
          queue: playerStateRef.current?.queue || [],
          current: playerStateRef.current?.current || {},
          playIndex: playerStateRef.current?.playIndex ?? 0,
        }

        // 2. Fetch fresh remote queue for this target device (read-only, never overwrite)
        dispatch(syncQueue(null, []))
        httpClient(`/api/output/queue?deviceId=${newId}`)
          .then(({ json }) => {
            if (json && json.queue && json.queue.length > 0) {
              syncChannelQueueToRedux(json, true)
            }
          })
          .catch(() => {})
          .finally(() => {
            setTimeout(() => {
              isSwitchingTargetRef.current = false
            }, 500)
          })

        // 3. Fetch latest remote playback status
        httpClient('/api/output/status')
          .then(({ json }) => {
            if (json) {
              remoteProgressRef.current.baseSec = Number(json.timeOffset) || 0
              remoteProgressRef.current.baseClientTime = Date.now()
              remoteProgressRef.current.playing = json.state === 'playing'
              remoteProgressRef.current.duration = Number(json.track?.duration) || 0
              remoteProgressRef.current.trackId = json.track?.trackId || null
              remoteProgressRef.current.currentIndex = json.currentIndex ?? -1
              if (audioInstanceRef.current) {
                isSyntheticEventRef.current = true
                if (json.state === 'playing') {
                  audioInstanceRef.current.dispatchEvent(new Event('play'))
                } else {
                  audioInstanceRef.current.dispatchEvent(new Event('pause'))
                }
                isSyntheticEventRef.current = false
              }
            }
          })
          .catch(() => {})
      } else {
        // Switching back from remote to local:
        isSwitchingTargetRef.current = true
        // 1. Reset remote virtual clock so remote progress is never inherited
        remoteProgressRef.current = {
          baseSec: 0,
          baseClientTime: 0,
          playing: false,
          duration: 0,
          trackId: null,
          currentIndex: -1,
        }

        if (audioInstanceRef.current) {
          audioInstanceRef.current.muted = false
        }

        // 2. Restore local queue snapshot if present
        if (localQueueSnapshotRef.current?.queue?.length > 0) {
          const snap = localQueueSnapshotRef.current
          dispatch(syncQueue(snap.current, snap.queue))
          setTimeout(() => {
            isSwitchingTargetRef.current = false
          }, 300)
        } else {
          // Restore from terminal queue on server
          httpClient('/api/player/terminal-queue')
            .then(({ json }) => {
              if (json && json.found && json.queue && json.queue.length > 0) {
                const idx = json.currentIndex >= 0 && json.currentIndex < json.queue.length ? json.currentIndex : 0
                const restoredQueue = json.queue.map((t) => ({
                  trackId: t.trackId,
                  uuid: t.trackId,
                  name: t.title,
                  singer: t.artist,
                  duration: t.duration,
                  cover: sanitizeImageUrl(t.cover || t.coverUrl || subsonic.getCoverArtUrl({ id: t.trackId }, 300)),
                  musicSrc: subsonic.streamUrl(t.trackId) || SILENT_AUDIO_URI,
                  song: {
                    id: t.trackId,
                    title: t.title,
                    artist: t.artist,
                    album: t.album,
                    duration: t.duration,
                  },
                }))
                dispatch(syncQueue(restoredQueue[idx], restoredQueue))
              }
            })
            .catch(() => {})
            .finally(() => {
              setTimeout(() => {
                isSwitchingTargetRef.current = false
              }, 300)
            })
        }
      }
    }
    window.addEventListener('nd:output-device-changed', handleDeviceChanged)
    return () => {
      window.removeEventListener('nd:output-device-changed', handleDeviceChanged)
    }
  }, [dispatch, syncChannelQueueToRedux])

  const ensureStatusPill = useCallback((timeEl, isRemote, deviceName, stateText) => {
    if (!timeEl || !timeEl.parentElement) return
    let pill = timeEl.parentElement.querySelector('.nd-stream-status-pill')
    if (!isRemote) {
      if (pill) pill.remove()
      return
    }
    if (!pill) {
      pill = document.createElement('span')
      pill.className = 'nd-stream-status-pill'
      pill.innerHTML = '<span class="nd-stream-status-dot"></span><span class="nd-stream-status-text"></span>'
      timeEl.insertAdjacentElement('afterend', pill)
    }
    const textEl = pill.querySelector('.nd-stream-status-text')
    if (textEl) {
      const current = playerStateRef.current?.current
      const song = current?.song
      const format = (song?.suffix || 'RAW').toUpperCase()
      const text = `${deviceName} · 源码直通 · ${format} · ${stateText}`
      if (textEl.innerText !== text) {
        textEl.innerText = text
      }
    }
  }, [])

  // Decouple remote output from browser lifecycle: handle tab visibility and window focus
  useEffect(() => {
    const handleVisibilityChange = () => {
      isHiddenRef.current = document.hidden
      if (!document.hidden && activeOutputDeviceRef.current !== 'local') {
        // Tab restored to foreground: re-align status silently without restarting stream
        httpClient('/api/output/status')
          .then(({ json }) => {
            if (json && json.state) {
              // Remote session is alive and playing autonomously on backend
            }
          })
          .catch(() => {})
      }
    }
    const handleBlur = () => {
      isHiddenRef.current = true
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    window.addEventListener('focus', handleVisibilityChange)
    window.addEventListener('blur', handleBlur)
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      window.removeEventListener('focus', handleVisibilityChange)
      window.removeEventListener('blur', handleBlur)
    }
  }, [])

  // Local Audio Playback Trigger & State Watcher
  // Solves the issue where ReactJkMusicPlayer skips calling audio.play() on track change / 0===0 collision
  useEffect(() => {
    if (!audioInstance) return

    if (activeOutputDevice !== 'local') {
      audioInstance.muted = true
      return
    }

    // Local device is active: ensure audio is unmuted
    audioInstance.muted = false

    const playSafe = (reason) => {
      if (activeOutputDeviceRef.current !== 'local') return
      const currentQueue = playerStateRef.current?.queue || []
      if (currentQueue.length === 0) return

      const src = audioInstance.src
      // Ensure audio has a valid media stream source, not empty, placeholder, or web page URL
      if (
        !src ||
        src === window.location.href ||
        src.endsWith('/') ||
        src.startsWith('data:') ||
        src.includes('placeholder')
      ) {
        return
      }

      audioInstance.muted = false
      if (audioInstance.paused) {
        const playPromise = audioInstance.play()
        if (playPromise && playPromise.then) {
          playPromise
            .then(() => {
              // Started successfully
            })
            .catch((err) => {
              if (
                err &&
                (err.name === 'AbortError' ||
                  err.name === 'NotSupportedError' ||
                  err.name === 'NotAllowedError')
              ) {
                return
              }
              console.warn(`[LocalAudio] Play deferred (${reason}):`, err)
            })
        }
      }
    }

    // User initiated track selection or queue reset
    const hasQueue = (playerState.queue || []).length > 0
    const shouldPlay =
      hasQueue &&
      playerState.autoPlay !== false &&
      (playerState.clear ||
        playerState.playIndex != null ||
        playerState.playIndex === 0)
    if (shouldPlay) {
      playSafe('track-state-change')
    }
  }, [
    audioInstance,
    activeOutputDevice,
    playerState.queue,
    playerState.playIndex,
    playerState.clear,
    playerState.autoPlay,
  ])
  useEffect(() => {
    // Scheme A: Local queue is completely autonomous on client, never synced to server as remote queue
    if (activeOutputDevice === 'local' || activeOutputDeviceRef.current === 'local') {
      return
    }
    // Block automatic queue overwrite when switching output target
    if (isSwitchingTargetRef.current) {
      return
    }
    if (!playerState.queue || !playerState.queue.length) return

    const targetDevice = activeOutputDeviceRef.current
    const currentIdx =
      playerState.playIndex ?? playerState.savedPlayIndex ?? 0
    const queueKey = `remote_${targetDevice}_${playerState.queue.map((t) => t.trackId).join(',')}_${currentIdx}_${playerState.mode}`
    if (lastSyncedQueueKeyRef.current === queueKey) return
    lastSyncedQueueKeyRef.current = queueKey

    const payloadTracks = playerState.queue.map((item) => ({
      trackId: item.trackId,
      title: item.song?.title || item.name || '',
      artist: item.song?.artist || item.singer || '',
      album: item.song?.album || '',
      duration: item.duration || item.song?.duration || 0,
      streamUrl: `/api/output/stream/${item.trackId}`,
    }))

    const isPlaying =
      audioInstance && !audioInstance.paused && !audioInstance.ended
    const shouldAutoPlay = Boolean(
      playerState.clear || playerState.playIndex != null || isPlaying,
    )

    if (audioInstance) {
      isSyntheticEventRef.current = true
      audioInstance.muted = true
      if (!audioInstance.paused) {
        audioInstance.pause()
      }
      isSyntheticEventRef.current = false
    }

    if (shouldAutoPlay && payloadTracks[currentIdx]) {
      const targetItem = payloadTracks[currentIdx]
      remoteProgressRef.current = {
        baseSec: 0,
        baseClientTime: Date.now(),
        playing: true,
        duration: targetItem.duration || 0,
        trackId: targetItem.trackId,
        currentIndex: currentIdx,
      }
      virtualSecRef.current = 0
      remoteSyncLockUntilRef.current = Date.now() + 2500
    }

    httpClient('/api/output/queue', {
      method: 'POST',
      body: JSON.stringify({
        channel: 'remote',
        deviceId: targetDevice,
        tracks: payloadTracks,
        startIndex: currentIdx,
        timeOffset: 0,
        playMode: playerState.mode,
        autoPlay: shouldAutoPlay,
      }),
    }).catch((err) => {
      remoteProgressRef.current.playing = false
      remoteSyncLockUntilRef.current = 0
      notify(
        `远端推流失败: ${err?.body?.message || err?.message || '请检查设备连接或推流配置'}`,
        'error',
      )
    })
  }, [
    activeOutputDevice,
    playerState.queue,
    playerState.playIndex,
    playerState.mode,
    playerState.clear,
    audioInstance,
  ])

  // Local mode: save terminal queue to server periodically (isolated by clientId)
  useEffect(() => {
    if (activeOutputDevice !== 'local' || activeOutputDeviceRef.current !== 'local') return
    if (!playerState.queue || playerState.queue.length === 0) return

    if (saveTerminalQueueTimerRef.current) {
      clearTimeout(saveTerminalQueueTimerRef.current)
    }
    saveTerminalQueueTimerRef.current = setTimeout(() => {
      const tracks = playerState.queue.map((t) => ({
        trackId: t.trackId,
        title: t.song?.title || t.name || '',
        artist: t.song?.artist || t.singer || '',
        album: t.song?.album || '',
        duration: t.duration || t.song?.duration || 0,
      }))
      const idx = playerState.playIndex ?? playerState.savedPlayIndex ?? 0
      const offset = audioInstanceRef.current?.currentTime || 0
      httpClient('/api/player/terminal-queue', {
        method: 'POST',
        body: JSON.stringify({
          queue: tracks,
          currentIndex: idx,
          timeOffset: offset,
        }),
      }).catch(() => {})
    }, 1000)
  }, [activeOutputDevice, playerState.queue, playerState.playIndex])

  // Sync state from server session every 2 seconds
  useInterval(
    () => {
      if (activeOutputDevice !== 'local') {
        httpClient('/api/output/status')
          .then(({ json }) => {
            if (json) {
              if (Date.now() < remoteSyncLockUntilRef.current) {
                return
              }
              const isServerPlaying = json.state === 'playing'
              remoteProgressRef.current.baseSec = Number(json.timeOffset) || 0
              remoteProgressRef.current.baseClientTime = Date.now()
              remoteProgressRef.current.playing = isServerPlaying
              remoteProgressRef.current.duration = Number(json.track?.duration) || 0
              remoteProgressRef.current.trackId = json.track?.trackId || null
              remoteProgressRef.current.currentIndex = json.currentIndex ?? -1

              // Synchronize play/pause button icon with server state only when it changes
              if (
                audioInstance &&
                lastSyncedRemotePlayingRef.current !== isServerPlaying
              ) {
                lastSyncedRemotePlayingRef.current = isServerPlaying
                isSyntheticEventRef.current = true
                if (isServerPlaying) {
                  audioInstance.dispatchEvent(new Event('play'))
                } else {
                  audioInstance.dispatchEvent(new Event('pause'))
                }
                isSyntheticEventRef.current = false
              }

              // When server switches to another track, update Redux state so UI updates
              const serverIdx = json.currentIndex
              const queue = playerStateRef.current?.queue
              if (
                serverIdx >= 0 &&
                queue &&
                queue[serverIdx] &&
                (playerStateRef.current.savedPlayIndex !== serverIdx ||
                  playerStateRef.current.current?.trackId !== json.track?.trackId)
              ) {
                const newTrack = queue[serverIdx]
                dispatch(currentPlaying(newTrack))
                currentTrackIdRef.current = newTrack.trackId
                setCurrentTrackId(newTrack.trackId)
                remoteProgressRef.current.baseSec = 0
                remoteProgressRef.current.baseClientTime = Date.now()
                virtualSecRef.current = 0
                if (audioInstance && typeof audioInstance.playByIndex === 'function') {
                  try {
                    audioInstance.playByIndex(serverIdx)
                  } catch (e) {
                    // ignore
                  }
                }
              }
            }
          })
          .catch(() => {})
      }
    },
    activeOutputDevice !== 'local' ? 2000 : null,
  )

  // Drive visual progress bar and time label forward smoothly every 500ms
  useInterval(
    () => {
      if (activeOutputDevice !== 'local') {
        const clock = remoteProgressRef.current
        let currentSec = clock.baseSec
        if (clock.playing && clock.baseClientTime > 0) {
          currentSec += Math.max(0, (Date.now() - clock.baseClientTime) / 1000)
        }
        if (clock.duration > 0 && currentSec > clock.duration) {
          currentSec = clock.duration
        }

        virtualSecRef.current = currentSec

        if (audioInstance) {
          try {
            audioInstance.dispatchEvent(new Event('timeupdate'))
          } catch (e) {
            // ignore
          }
        }

        const formatTime = (seconds) => {
          const s = Math.floor(seconds || 0)
          const m = Math.floor(s / 60)
          const sec = s % 60
          return `${m < 10 ? '0' + m : m}:${sec < 10 ? '0' + sec : sec}`
        }
        const timeEls = document.querySelectorAll('.current-time, .music-player-panel .current-time')
        const devName = activeDeviceNameRef.current || (activeOutputDevice === 'hqplayer' ? 'HQPlayer' : '远端设备')
        const stateText = clock.playing ? '推流中' : '已暂停'
        timeEls.forEach((el) => {
          el.innerText = formatTime(currentSec)
          ensureStatusPill(el, true, devName, stateText)
        })
        const durEls = document.querySelectorAll('.duration, .music-player-panel .duration')
        if (clock.duration > 0) {
          durEls.forEach((el) => {
            el.innerText = formatTime(clock.duration)
          })
        }
        const trackEls = document.querySelectorAll('.rc-slider-track, .progress-bar .rc-slider-track')
        const handleEls = document.querySelectorAll('.rc-slider-handle, .progress-bar .rc-slider-handle')
        const isDragging = document.querySelector('.rc-slider-handle-dragging') ||
          document.querySelector('.rc-slider-handle:active')
        if (!isDragging && clock.duration > 0) {
          const pct = Math.min(100, Math.max(0, (currentSec / clock.duration) * 100))
          trackEls.forEach((el) => {
            el.style.width = `${pct}%`
          })
          handleEls.forEach((el) => {
            el.style.left = `${pct}%`
            el.setAttribute('aria-valuenow', Math.floor(currentSec))
          })
        }
      }
    },
    activeOutputDevice !== 'local' ? 500 : null,
  )

  useEffect(() => {
    if (activeOutputDevice === 'local') {
      document.querySelectorAll('.nd-stream-status-pill').forEach((el) => el.remove())
    }
  }, [activeOutputDevice])

  const visible = authenticated
  const placeholderTracks = useMemo(
    () => [
      {
        name: 'Navidrome 就绪',
        singer: '等待播放曲目',
        cover: '/app-icon.png',
        musicSrc: SILENT_AUDIO_URI,
        isPlaceholder: true,
      },
    ],
    [],
  )
  const isRadio = playerState.current?.isRadio || false
  const classes = useStyle({
    isRadio,
    visible,
    enableCoverAnimation: config.enableCoverAnimation,
  })
  const showNotifications = useSelector(
    (state) => state.settings.notifications || false,
  )
  const gainInfo = useSelector((state) => state.replayGain)

  useEffect(() => {
    if (
      !isMobilePlayer &&
      context === null &&
      audioInstance &&
      config.enableReplayGain &&
      'AudioContext' in window &&
      (gainInfo.gainMode === 'album' || gainInfo.gainMode === 'track')
    ) {
      const ctx = new AudioContext()
      // we need this to support radios in firefox
      audioInstance.crossOrigin = 'anonymous'
      const source = ctx.createMediaElementSource(audioInstance)
      const gain = ctx.createGain()

      source.connect(gain)
      gain.connect(ctx.destination)

      setContext(ctx)
      setGainNode(gain)
    }
  }, [audioInstance, context, gainInfo.gainMode])

  useEffect(() => {
    if (gainNode) {
      const current = playerState.current || {}
      const song = current.song || {}

      const numericGain = calculateGain(gainInfo, song)
      gainNode.gain.setValueAtTime(numericGain, context.currentTime)
    }
  }, [audioInstance, context, gainNode, playerState, gainInfo])

  useEffect(() => {
    const handleBeforeUnload = (e) => {
      if (playerState.current?.uuid && audioInstance && !audioInstance.paused) {
        e.preventDefault()
        e.returnValue = ''
      }
    }

    const handlePageHide = () => {
      // If audio is currently playing, this is a background transition (lock screen or app switch), not a genuine unload
      if (audioInstance && !audioInstance.paused) {
        return
      }
      if (currentTrackIdRef.current && !playerState.current?.isRadio) {
        stoppedRef.current = true
        try {
          subsonic.reportPlaybackKeepalive(
            currentTrackIdRef.current,
            lastPositionMsRef.current,
            'stopped',
          )
        } catch {
          // fetch/sendBeacon may throw; ignore
        }
      }
    }

    window.addEventListener('beforeunload', handleBeforeUnload)
    window.addEventListener('pagehide', handlePageHide)
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
      window.removeEventListener('pagehide', handlePageHide)
    }
  }, [playerState, audioInstance])

  const defaultOptions = useMemo(
    () => ({
      theme: playerTheme,
      preload: 'auto',
      bounds: 'body',
      playMode: playerState.mode,
      mode: 'full',
      loadAudioErrorPlayNext: false,
      autoPlayInitLoadPlayList: true,
      clearPriorAudioLists: false,
      showDestroy: false,
      showDownload: false,
      showLyric: true,
      showReload: false,
      toggleMode: false,
      showMiniProcessBar: false,
      drag: false,
      responsive: false,
      glassBg: false,
      showThemeSwitch: false,
      showMediaSession: true,
      restartCurrentOnPrev: true,
      quietUpdate: true,
      defaultPosition: {
        top: 300,
        left: 120,
      },
      volumeFade: { fadeIn: 200, fadeOut: 200 },
      renderAudioTitle: (audioInfo, isMobile) => (
        <AudioTitle
          audioInfo={audioInfo}
          gainInfo={gainInfo}
          isMobile={isMobile}
        />
      ),
      locale: locale(translate),
      sortableOptions: { delay: 200, delayOnTouchOnly: true },
    }),
    [gainInfo, isDesktop, playerTheme, translate, playerState.mode],
  )

  const formatDurationSec = (sec) => {
    if (!sec || isNaN(sec)) return ''
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m < 10 ? '0' : ''}${m}:${s < 10 ? '0' : ''}${s}`
  }

  const options = useMemo(() => {
    const current = playerState.current || {}
    const hasQueue = playerState.queue.length > 0
    const activePlayIndex =
      playerState.playIndex != null
        ? playerState.playIndex
        : playerState.savedPlayIndex != null && playerState.savedPlayIndex >= 0
        ? playerState.savedPlayIndex
        : undefined
    return {
      ...defaultOptions,
      audioLists: hasQueue
        ? playerState.queue.map((item) => {
            const dur = formatDurationSec(item.duration || item.song?.duration)
            const rawSinger =
              item.song?.artist ||
              (typeof item.singer === 'string'
                ? item.singer.split(' · ')[0]
                : '') ||
              '未知歌手'
            return {
              ...item,
              singer: dur ? `${rawSinger} · ${dur}` : rawSinger,
              musicSrc:
                activeOutputDevice !== 'local'
                  ? SILENT_AUDIO_URI
                  : item.musicSrc || SILENT_AUDIO_URI,
            }
          })
        : placeholderTracks,
      playIndex: hasQueue ? activePlayIndex : 0,
      autoPlay:
        hasQueue &&
        playerState.autoPlay !== false &&
        (playerState.clear || playerState.playIndex != null || playerState.playIndex === 0),
      clearPriorAudioLists: playerState.clear,
      extendsContent: (
        <PlayerToolbar id={current.trackId} isRadio={current.isRadio} />
      ),
      defaultVolume: isMobilePlayer ? 1 : playerState.volume,
      showMediaSession: !current.isRadio && hasQueue,
    }
  }, [playerState, defaultOptions, isMobilePlayer, placeholderTracks, activeOutputDevice])

  const onAudioListsChange = useCallback(
    (_, audioLists, audioInfo) => dispatch(syncQueue(audioInfo, audioLists)),
    [dispatch],
  )

  const onAudioProgress = useCallback((info) => {
    if (info.ended) {
      document.title = 'Navidrome'
    }
    if (!info.isRadio && info.currentTime != null) {
      lastPositionMsRef.current = Math.floor(info.currentTime * 1000)
    }
  }, [])

  const onAudioVolumeChange = useCallback(
    // sqrt to compensate for the logarithmic volume
    (volume) => {
      dispatch(setVolume(Math.sqrt(volume)))
      if (activeOutputDeviceRef.current !== 'local') {
        httpClient('/api/output/control', {
          method: 'POST',
          body: JSON.stringify({
            channel: 'remote',
            deviceId: activeOutputDeviceRef.current,
            action: 'volume',
            volume: Math.round(volume * 100),
          }),
        }).catch(() => {})
      }
    },
    [dispatch],
  )

  const onPlayIndexChange = useCallback(
    (playIndex) => {
      if (activeOutputDeviceRef.current !== 'local') {
        const queue = playerStateRef.current?.queue
        if (queue && queue[playIndex]) {
          const targetTrack = queue[playIndex]
          currentTrackIdRef.current = targetTrack.trackId
          setCurrentTrackId(targetTrack.trackId)
          dispatch(currentPlaying(targetTrack))

          if (
            Date.now() >= remoteSyncLockUntilRef.current &&
            remoteProgressRef.current.currentIndex !== playIndex
          ) {
            remoteProgressRef.current.currentIndex = playIndex
            remoteProgressRef.current.trackId = targetTrack.trackId
            remoteProgressRef.current.baseSec = 0
            remoteProgressRef.current.baseClientTime = Date.now()
            remoteProgressRef.current.duration = targetTrack.duration || 0
            remoteProgressRef.current.playing = true
            virtualSecRef.current = 0
            remoteSyncLockUntilRef.current = Date.now() + 2500

            httpClient('/api/output/control', {
              method: 'POST',
              body: JSON.stringify({
                channel: 'remote',
                deviceId: activeOutputDeviceRef.current,
                action: 'jump',
                index: playIndex,
              }),
            }).catch(() => {})
          }
        }
      }
    },
    [dispatch],
  )

  const onAudioPlay = useCallback(
    (info) => {
      if (isSyntheticEventRef.current) {
        return
      }

      if (activeOutputDeviceRef.current !== 'local') {
        if (audioInstance) {
          audioInstance.muted = true
          if (!audioInstance.paused) {
            audioInstance.pause()
          }
        }
        return
      }

      isUserExplicitlyPausedRef.current = false
      startSilentKeepAlive()

      if (context && context.state !== 'running') {
        context.resume()
      }

      dispatch(currentPlaying(info))
      if (info.isPlaceholder) return
      recordRecentlyPlayed(info)

      if (info.duration) {
        const song = info.song || {}
        const sTitle = song.title || info.name || ''
        const sArtist = song.artist || info.singer || ''
        document.title = `${sTitle} - ${sArtist} - Navidrome`
        if (!info.isRadio) {
          const isNewTrack = info.trackId !== currentTrackIdRef.current
          if (activeOutputDeviceRef.current === 'local') {
            if (isNewTrack) {
              if (audioInstance && audioInstance.currentTime > 0) {
                audioInstance.currentTime = 0
              }
              lastPositionMsRef.current = 0
              currentTrackIdRef.current = info.trackId
              setCurrentTrackId(info.trackId)
              subsonic
                .reportPlayback(info.trackId, 0, 'starting')
                .then(() =>
                  subsonic.reportPlayback(info.trackId, 0, 'playing'),
                )
                .catch(() => {})
            } else {
              const posMs = Math.floor((info.currentTime || 0) * 1000)
              lastPositionMsRef.current = posMs
              subsonic
                .reportPlayback(info.trackId, posMs, 'playing')
                .catch(() => {})
            }
            setHeartbeatTrackId(info.trackId)
          } else {
            lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
          }
        }
        if (config.gaTrackingId) {
          ReactGA.event({
            category: 'Player',
            action: 'Play song',
            label: `${sTitle} - ${sArtist}`,
          })
        }
        if (showNotifications) {
          sendNotification(
            sTitle,
            `${sArtist} - ${song.album || ''}`,
            info.cover,
          )
        }
      }
    },
    [context, dispatch, showNotifications, currentTrackId],
  )

  const onAudioPlayTrackChange = useCallback(() => {
    if (activeOutputDeviceRef.current === 'local') {
      if (currentTrackId) {
        subsonic
          .reportPlayback(currentTrackId, lastPositionMsRef.current, 'stopped')
          .catch(() => {})
      }
      lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
    }
  }, [currentTrackId])

  const onAudioPause = useCallback(
    (info) => {
      if (activeOutputDeviceRef.current !== 'local') {
        return
      }

      const isRecentUserInteraction =
        document.visibilityState === 'visible' &&
        Date.now() - lastUserInteractionTimeRef.current < 1200

      if (isRecentUserInteraction) {
        isUserExplicitlyPausedRef.current = true
        logAudioGuard(
          `[Audio:pause] User clicked pause button in UI. trackId=${currentTrackIdRef.current}`,
        )
        stopSilentKeepAlive()
      } else {
        logAudioGuard(
          `[Audio:pause] Passive pause detected (screen off, wireless charging, or network drop). isUserExplicitlyPaused remains false.`,
        )
      }

      dispatch(currentPlaying(info))
      if (isSyntheticEventRef.current) {
        return
      }

      // When remote output is active:
      // STRICT GUARD: If tab is hidden/minimized/in background, or screen is sleeping,
      // the pause event is a synthetic browser power-saving throttling event, NOT a user intent!
      // Do NOT send pause to remote hardware in background!
      if (activeOutputDeviceRef.current !== 'local' && !info.isPlaceholder) {
        const wasPlaying = remoteProgressRef.current.playing
        const elapsed =
          remoteProgressRef.current.baseClientTime > 0
            ? (Date.now() - remoteProgressRef.current.baseClientTime) / 1000
            : 0
        remoteProgressRef.current.baseSec += Math.max(0, elapsed)
        remoteProgressRef.current.baseClientTime = Date.now()
        remoteProgressRef.current.playing = false
        if (wasPlaying && !document.hidden && !isHiddenRef.current) {
          httpClient('/api/output/control', {
            method: 'POST',
            body: JSON.stringify({
              channel: 'remote',
              deviceId: activeOutputDeviceRef.current,
              action: 'pause',
            }),
          }).catch(() => {})
        }
      }
      if (
        activeOutputDeviceRef.current === 'local' &&
        !info.isRadio &&
        currentTrackId
      ) {
        const posMs = Math.floor(info.currentTime * 1000)
        lastPositionMsRef.current = posMs
        subsonic
          .reportPlayback(currentTrackId, posMs, 'paused')
          .catch(() => {})
      }
      lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
    },
    [dispatch, currentTrackId, stopSilentKeepAlive],
  )

  useEffect(() => {
    const handleForceLocalPause = () => {
      isUserExplicitlyPausedRef.current = true
      needsNetworkResumeRef.current = false
      stopSilentKeepAlive()
      const audio = audioInstanceRef.current
      if (audio && !audio.paused) {
        try {
          audio.pause()
        } catch (e) {}
      }
      logAudioGuard('[Lifecycle:force-local-pause] Locked paused before external browser switch')
    }
    window.addEventListener('nd:force-local-pause', handleForceLocalPause)
    return () => {
      window.removeEventListener('nd:force-local-pause', handleForceLocalPause)
    }
  }, [stopSilentKeepAlive])

  const onAudioEnded = useCallback(
    (currentPlayId, audioLists, info) => {
      if (info.isPlaceholder) return
      if (activeOutputDeviceRef.current !== 'local') return
      if (currentTrackId && !info.isRadio) {
        const posMs = Math.floor((info.duration || 0) * 1000)
        subsonic
          .reportPlayback(currentTrackId, posMs, 'stopped')
          .catch(() => {})
      }
      lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(currentPlaying(info))
      dataProvider
        .getOne('keepalive', { id: info.trackId })
        .catch(() => {})
    },
    [dispatch, dataProvider, currentTrackId],
  )

  const onCoverClick = useCallback((mode, audioLists, audioInfo) => {
    if (mode === 'full' && audioInfo?.song?.albumId) {
      window.location.href = `#/album/${audioInfo.song.albumId}/show`
    }
  }, [])

  const onAudioError = useCallback(
    (error, currentPlayId, audioLists, audioInfo) => {
      // Invalidate all cached decisions — token may be stale
      decisionService.invalidateAll()

      // Pre-fetch decisions for upcoming songs with fresh tokens
      const currentIdx = playerState.queue.findIndex(
        (item) => item.uuid === currentPlayId,
      )
      if (currentIdx >= 0) {
        const nextSongIds = playerState.queue
          .slice(currentIdx + 1, currentIdx + 4)
          .filter(
            (item) =>
              !item.isRadio &&
              item.trackId &&
              !item.trackId.startsWith('online:'),
          )
          .map((item) => item.trackId)
        if (nextSongIds.length > 0) {
          decisionService.prefetchDecisions(nextSongIds)
        }
      }
    },
    [playerState.queue],
  )

  const lastLyricTextRef = useRef('')
  const lastLyricUpdateAtRef = useRef(0)

  const onAudioLyricChange = useCallback((_lineNum, txt) => {
    if (activeOutputDeviceRef.current !== 'local') return
    if (
      typeof navigator === 'undefined' ||
      !('mediaSession' in navigator) ||
      typeof window.MediaMetadata === 'undefined'
    ) {
      return
    }
    const cleanLyric = (txt || '').trim()
    if (cleanLyric === lastLyricTextRef.current) return

    const now = Date.now()
    // Throttle updates to >= 800ms to avoid Bluetooth AVRCP flooding
    if (cleanLyric && now - lastLyricUpdateAtRef.current < 800) {
      return
    }

    const currentMeta = navigator.mediaSession.metadata
    const current = playerStateRef.current?.current
    const baseTitle =
      current?.name || current?.song?.title || currentMeta?.title || ''
    const baseArtist =
      current?.song?.artist ||
      (typeof current?.singer === 'string'
        ? current.singer.split(' · ')[0]
        : '') ||
      currentMeta?.artist?.split(' | ')[0] ||
      ''
    const baseAlbum = current?.song?.album || currentMeta?.album || baseTitle

    if (!baseTitle) return

    lastLyricTextRef.current = cleanLyric
    lastLyricUpdateAtRef.current = now

    try {
      const nextArtist = cleanLyric
        ? baseArtist
          ? `${baseArtist} | ${cleanLyric}`
          : cleanLyric
        : baseArtist

      navigator.mediaSession.metadata = new window.MediaMetadata({
        title: baseTitle,
        artist: nextArtist,
        album: baseAlbum,
        artwork:
          currentMeta?.artwork ||
          (current?.cover ? [{ src: current.cover }] : []),
      })
    } catch (e) {
      // Ignore MediaMetadata errors on unsupported WebViews
    }
  }, [])

  const onBeforeDestroy = useCallback(() => {
    return new Promise((resolve, reject) => {
      if (currentTrackId && !playerStateRef.current?.current?.isRadio) {
        subsonic
          .reportPlayback(
            currentTrackId,
            lastPositionMsRef.current,
            'stopped',
          )
          .catch(() => {})
      }
      lastLyricTextRef.current = ''
      lastLyricUpdateAtRef.current = 0
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(clearQueue())
      reject()
    })
  }, [dispatch, currentTrackId])

  if (!visible) {
    document.title = 'Navidrome'
  }

  const handleImportLocalQueueToRemote = useCallback(() => {
    const localSnap = localQueueSnapshotRef.current
    if (!localSnap || !localSnap.queue || localSnap.queue.length === 0) {
      notify('本机暂无可导入的播放列表', 'warning')
      return
    }
    const targetDev = activeOutputDeviceRef.current
    const devName = activeDeviceNameRef.current || '远端设备'
    const confirm = window.confirm(
      `是否将本机列表 (${localSnap.queue.length} 首歌曲) 推送到 ${devName} 并立即播放？`,
    )
    if (!confirm) return

    const payloadTracks = localSnap.queue.map((item) => ({
      trackId: item.trackId,
      title: item.song?.title || item.name || '',
      artist: item.song?.artist || item.singer || '',
      album: item.song?.album || '',
      duration: item.duration || item.song?.duration || 0,
      streamUrl: `/api/output/stream/${item.trackId}`,
    }))

    const startIdx = localSnap.playIndex || 0
    httpClient('/api/output/queue', {
      method: 'POST',
      body: JSON.stringify({
        channel: 'remote',
        deviceId: targetDev,
        tracks: payloadTracks,
        startIndex: startIdx,
        timeOffset: 0,
        playMode: playerState.mode || 'orderLoop',
        autoPlay: true,
      }),
    })
      .then(() => {
        notify(`已成功将本机列表推送到 ${devName}`, 'success')
        httpClient(`/api/output/queue?deviceId=${targetDev}`)
          .then(({ json }) => {
            if (json && json.queue) syncChannelQueueToRedux(json, true)
          })
          .catch(() => {})
      })
      .catch((err) => {
        notify(`导入失败: ${err.message}`, 'error')
      })
  }, [notify, playerState.mode, syncChannelQueueToRedux])

  const handlers = useMemo(
    () => keyHandlers(audioInstance, playerState),
    [audioInstance, playerState],
  )

  useEffect(() => {
    if (isMobilePlayer && audioInstance) {
      audioInstance.volume = 1
    }
  }, [isMobilePlayer, audioInstance])

  // MediaSession position state, playback state & mobile background resume breakpoint recovery
  useEffect(() => {
    if (!audioInstance) return

    // Update MediaSession Position State when playback timing changes
    const updatePositionState = () => {
      if (
        'mediaSession' in navigator &&
        'setPositionState' in navigator.mediaSession &&
        audioInstance.duration &&
        !isNaN(audioInstance.duration) &&
        isFinite(audioInstance.duration)
      ) {
        try {
          navigator.mediaSession.setPositionState({
            duration: audioInstance.duration,
            playbackRate: audioInstance.playbackRate || 1,
            position: Math.min(
              audioInstance.currentTime || 0,
              audioInstance.duration,
            ),
          })
        } catch (e) {
          // Ignore transient errors on rapid state transitions
        }
      }
    }

    let wakeLockSentinel = null
    const acquireWakeLock = async () => {
      if ('wakeLock' in navigator && !wakeLockSentinel) {
        try {
          wakeLockSentinel = await navigator.wakeLock.request('screen')
          wakeLockSentinel.addEventListener('release', () => {
            wakeLockSentinel = null
          })
        } catch (e) {}
      }
    }
    const releaseWakeLock = () => {
      if (wakeLockSentinel) {
        try {
          wakeLockSentinel.release()
        } catch (e) {}
        wakeLockSentinel = null
      }
    }

    const handlePlay = () => {
      if (isSyntheticEventRef.current || activeOutputDeviceRef.current !== 'local') {
        return
      }
      isUserPausedRef.current = false
      isUserExplicitlyPausedRef.current = false
      needsNetworkResumeRef.current = false
      watchdogStallCountRef.current = 0
      if (currentTrackIdRef.current) {
        trackErrorCountRef.current[currentTrackIdRef.current] = 0
      }
      logAudioGuard(
        `[Audio:play] Started. trackId=${currentTrackIdRef.current}, currentTime=${audioInstance?.currentTime?.toFixed(2)}s, network=${navigator.onLine ? 'online' : 'offline'}`,
      )
      if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'playing'
      }
      acquireWakeLock()
      startSilentKeepAlive()
    }

    const handlePause = () => {
      if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'paused'
      }
      releaseWakeLock()

      // Hardware / wireless charging / system audio session interruption recovery:
      // If audio was paused by external interruption without user clicking pause, auto-resume after 1500ms
      if (
        activeOutputDeviceRef.current === 'local' &&
        !isUserExplicitlyPausedRef.current &&
        !audioInstance.ended &&
        currentTrackIdRef.current
      ) {
        logAudioGuard(
          `[LocalAudio] Entering passive interruption recovery queue (1500ms)... currentTime=${audioInstance?.currentTime?.toFixed(2)}s`,
        )
        if (interruptionTimerRef.current) clearTimeout(interruptionTimerRef.current)
        interruptionTimerRef.current = setTimeout(() => {
          if (
            activeOutputDeviceRef.current === 'local' &&
            !isUserExplicitlyPausedRef.current &&
            audioInstance &&
            audioInstance.paused &&
            !audioInstance.ended &&
            currentTrackIdRef.current
          ) {
            logAudioGuard(
              '[LocalAudio] Passive interruption: attempting audioInstance.play() after 1500ms...',
            )
            const p = audioInstance.play()
            if (p && p.then) {
              p.then(() => {
                logAudioGuard(
                  '[LocalAudio] Passive interruption successfully auto-resumed!',
                )
                needsNetworkResumeRef.current = false
              }).catch((err) => {
                if (err.name === 'NotAllowedError') {
                  warnAudioGuard(
                    '[LocalAudio] Audio focus permanently revoked (call or external media). Fusing to paused state:',
                    err,
                  )
                  // One-shot failure breaker: do NOT retry in a loop!
                  isUserExplicitlyPausedRef.current = true
                  isUserPausedRef.current = true
                  stopSilentKeepAlive()
                } else {
                  // Network drop, AbortError, or decode error
                  warnAudioGuard(
                    '[LocalAudio] Passive auto-resume delayed by network/system drop. Marking needsNetworkResume:',
                    err,
                  )
                  needsNetworkResumeRef.current = true
                  if (audioInstance.currentTime > 0) {
                    lastPositionMsRef.current = Math.floor(
                      audioInstance.currentTime * 1000,
                    )
                  }
                }
              })
            }
          }
        }, 1500)
      } else {
        stopSilentKeepAlive()
        isUserPausedRef.current = true
      }
    }

    const handleEnded = () => {
      isUserPausedRef.current = false
      isUserExplicitlyPausedRef.current = false
      // Keep silent keepalive running across song transition to prevent OS freezing JS thread!
      if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'playing'
      }
    }

    const handleTimeUpdate = () => {
      if (audioInstance.currentTime != null && !isNaN(audioInstance.currentTime)) {
        lastPositionMsRef.current = Math.floor(audioInstance.currentTime * 1000)
        // If track has played smoothly for >= 3 seconds, reset the consecutive skip avalanche counter!
        if (audioInstance.currentTime >= 3.0 && consecutiveSkipCountRef.current > 0) {
          consecutiveSkipCountRef.current = 0
        }
        if (currentTrackIdRef.current) {
          try {
            sessionStorage.setItem(
              'nd_pos_' + currentTrackIdRef.current,
              String(audioInstance.currentTime),
            )
          } catch (e) {}
        }
      }
      updatePositionState()
    }

    // Recover playback position on metadata loaded with one-shot lock
    const handleRestorePosition = () => {
      const target = pendingSeekTargetRef.current
      pendingSeekTargetRef.current = null // 一次性立即消费，清空锁
      if (target != null && target > 0) {
        if (
          audioInstance &&
          audioInstance.duration &&
          target < audioInstance.duration - 2 &&
          Math.abs((audioInstance.currentTime || 0) - target) > 1.5
        ) {
          try {
            logAudioGuard(
              `[Audio:restorePosition] One-shot seeking to ${target.toFixed(2)}s`,
            )
            audioInstance.currentTime = target
          } catch (e) {}
        }
      }
    }

    // Recover playback position on mobile Webview/tab resume if stream stalled or reset to 0
    const handleVisibilityOrResume = (evt) => {
      const evtType = evt?.type || 'visibilitychange'
      logAudioGuard(
        `[Lifecycle:${evtType}] Event fired: visibilityState=${document.visibilityState}, onLine=${navigator.onLine}, needsNetworkResume=${needsNetworkResumeRef.current}, userPaused=${isUserExplicitlyPausedRef.current}`,
      )

      if (document.visibilityState === 'visible' && audioInstance) {
        const needsResume =
          activeOutputDeviceRef.current === 'local' &&
          !isUserExplicitlyPausedRef.current &&
          currentTrackIdRef.current &&
          !audioInstance.ended &&
          (audioInstance.paused ||
            needsNetworkResumeRef.current ||
            audioInstance.error)

        if (needsResume) {
          const targetSec = (lastPositionMsRef.current || 0) / 1000
          logAudioGuard(
            `[Lifecycle] Restoring playback after visibility/online event at ${targetSec.toFixed(2)}s...`,
          )
          needsNetworkResumeRef.current = false
          if (
            audioInstance.error ||
            !audioInstance.src ||
            audioInstance.networkState === 3
          ) {
            reanchorAudioStream(audioInstance, targetSec)
          } else {
            const p = audioInstance.play()
            if (p && p.then) {
              p.catch((err) => {
                warnAudioGuard(
                  '[Lifecycle] Direct play failed, attempting stream re-anchor:',
                  err,
                )
                reanchorAudioStream(audioInstance, targetSec)
              })
            }
          }
        }

        if (!audioInstance.paused) {
          acquireWakeLock()
          startSilentKeepAlive()
        }
        if (
          audioInstance.currentTime === 0 &&
          (lastPositionMsRef.current || 0) > 2000
        ) {
          pendingSeekTargetRef.current = (lastPositionMsRef.current || 0) / 1000
          handleRestorePosition()
        }
      }
    }

    const handleStalled = () => {
      warnAudioGuard(
        `[Audio:stalled] Buffer stalled at ${audioInstance?.currentTime?.toFixed(2)}s, readyState=${audioInstance?.readyState}, networkState=${audioInstance?.networkState}`,
      )
    }

    const handleWaiting = () => {
      logAudioGuard(
        `[Audio:waiting] Waiting for media buffer at ${audioInstance?.currentTime?.toFixed(2)}s`,
      )
    }

    const handleError = () => {
      const err = audioInstance?.error
      const errCode = err?.code
      warnAudioGuard(
        `[Audio:error] code=${errCode}, message=${err?.message}`,
      )
      if (
        activeOutputDeviceRef.current === 'local' &&
        !isUserExplicitlyPausedRef.current &&
        currentTrackIdRef.current
      ) {
        const curTrackId = currentTrackIdRef.current
        const timeSinceReanchor = Date.now() - (lastReanchorTimeRef.current || 0)
        const isFastFail =
          lastReanchorTimeRef.current > 0 && timeSinceReanchor < 500
        const isBackgroundOrOffline =
          document.visibilityState === 'hidden' || !navigator.onLine

        // If error occurs while in background / offline or failed immediately (< 500ms) after reanchor:
        // Do NOT waste retry quota or trigger background auto-skip!
        // Lock breakpoint & mark needsNetworkResumeRef so it resumes cleanly upon visibility/online recovery.
        if (isBackgroundOrOffline || isFastFail) {
          const bpSec = (lastPositionMsRef.current || 0) / 1000
          if (bpSec > 0) {
            pendingSeekTargetRef.current = bpSec
          }
          needsNetworkResumeRef.current = true
          warnAudioGuard(
            `[Audio:error] Background/offline/fast-fail (${timeSinceReanchor}ms) detected (code=${errCode}). Preserving retry quota, locking breakpoint=${bpSec.toFixed(2)}s, awaiting visibility/online recovery.`,
          )
          return
        }

        const retries = trackErrorCountRef.current[curTrackId] || 0

        // If error code is 4 (format error) or 2 (network error)
        if ((errCode === 4 || errCode === 2) && retries < 1) {
          trackErrorCountRef.current[curTrackId] = retries + 1
          warnAudioGuard(
            `[Audio:error] Stream format/network error (code=${errCode}) on track ${curTrackId}. Auto-retry (1/1) in 1000ms...`,
          )
          setTimeout(() => {
            if (
              currentTrackIdRef.current === curTrackId &&
              audioInstance &&
              activeOutputDeviceRef.current === 'local' &&
              !isUserExplicitlyPausedRef.current
            ) {
              if (document.visibilityState === 'hidden' || !navigator.onLine) {
                needsNetworkResumeRef.current = true
                return
              }
              reanchorAudioStream(
                audioInstance,
                (lastPositionMsRef.current || 0) / 1000,
              )
            }
          }, 1000)
        } else if (retries >= 1 || errCode === 4) {
          // Retry failed or unrecoverable format error -> auto skip to next track with avalanche guard
          if (consecutiveSkipCountRef.current >= 3) {
            warnAudioGuard(
              `[Audio:error] Consecutive skip limit reached (3 tracks failed in a row). Stopping auto-skip to prevent queue avalanche. Marking needsNetworkResumeRef.`,
            )
            needsNetworkResumeRef.current = true
            trackErrorCountRef.current[curTrackId] = 0
          } else {
            consecutiveSkipCountRef.current += 1
            warnAudioGuard(
              `[Audio:error] Track ${curTrackId} failed permanently. Smoothly skipping to next track (consecutive skips: ${consecutiveSkipCountRef.current}/3)...`,
            )
            delete trackErrorCountRef.current[curTrackId]
            if (typeof audioInstance?.playNext === 'function') {
              audioInstance.playNext()
            }
          }
        }
      }
    }

    // Register full MediaSession action handlers
    const mediaSessionActions = [
      [
        'play',
        () => {
          logAudioGuard('[MediaSession:play] Lockscreen play triggered')
          isUserExplicitlyPausedRef.current = false
          needsNetworkResumeRef.current = false
          startSilentKeepAlive()
          audioInstance.play().catch(() => {})
        },
      ],
      [
        'pause',
        () => {
          logAudioGuard('[MediaSession:pause] Lockscreen pause triggered')
          isUserExplicitlyPausedRef.current = true
          stopSilentKeepAlive()
          audioInstance.pause()
        },
      ],
      [
        'previoustrack',
        () => {
          logAudioGuard('[MediaSession:previoustrack] Prev track triggered')
          isUserExplicitlyPausedRef.current = false
          needsNetworkResumeRef.current = false
          if (typeof audioInstance.playPrev === 'function') {
            audioInstance.playPrev()
          }
        },
      ],
      [
        'nexttrack',
        () => {
          logAudioGuard('[MediaSession:nexttrack] Next track triggered')
          isUserExplicitlyPausedRef.current = false
          needsNetworkResumeRef.current = false
          if (typeof audioInstance.playNext === 'function') {
            audioInstance.playNext()
          }
        },
      ],
      [
        'seekto',
        (details) => {
          if (details && details.seekTime != null) {
            audioInstance.currentTime = details.seekTime
          }
        },
      ],
      [
        'seekforward',
        (details) => {
          audioInstance.currentTime = Math.min(
            audioInstance.duration || Infinity,
            (audioInstance.currentTime || 0) + (details?.seekOffset || 10),
          )
        },
      ],
      [
        'seekbackward',
        (details) => {
          audioInstance.currentTime = Math.max(
            0,
            (audioInstance.currentTime || 0) - (details?.seekOffset || 10),
          )
        },
      ],
    ]

    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = audioInstance.paused
        ? 'paused'
        : 'playing'
      mediaSessionActions.forEach(([action, handler]) => {
        try {
          navigator.mediaSession.setActionHandler(action, handler)
        } catch (e) {}
      })
    }

    audioInstance.addEventListener('play', handlePlay)
    audioInstance.addEventListener('pause', handlePause)
    audioInstance.addEventListener('ended', handleEnded)
    audioInstance.addEventListener('timeupdate', handleTimeUpdate)
    audioInstance.addEventListener('durationchange', updatePositionState)
    audioInstance.addEventListener('loadedmetadata', handleRestorePosition)
    audioInstance.addEventListener('stalled', handleStalled)
    audioInstance.addEventListener('waiting', handleWaiting)
    audioInstance.addEventListener('error', handleError)
    document.addEventListener('visibilitychange', handleVisibilityOrResume)
    window.addEventListener('online', handleVisibilityOrResume)
    window.addEventListener('offline', handleVisibilityOrResume)

    return () => {
      audioInstance.removeEventListener('play', handlePlay)
      audioInstance.removeEventListener('pause', handlePause)
      audioInstance.removeEventListener('ended', handleEnded)
      audioInstance.removeEventListener('timeupdate', handleTimeUpdate)
      audioInstance.removeEventListener('durationchange', updatePositionState)
      audioInstance.removeEventListener('loadedmetadata', handleRestorePosition)
      audioInstance.removeEventListener('stalled', handleStalled)
      audioInstance.removeEventListener('waiting', handleWaiting)
      audioInstance.removeEventListener('error', handleError)
      document.removeEventListener('visibilitychange', handleVisibilityOrResume)
      window.removeEventListener('online', handleVisibilityOrResume)
      window.removeEventListener('offline', handleVisibilityOrResume)
    }
  }, [audioInstance])

  // Report every seek (including programmatic ones the library does not surface
  // via onAudioSeeked, e.g. restartCurrentOnPrev). Debounce coalesces drag
  // bursts into one report at the final position.
  useEffect(() => {
    if (!audioInstance) return
    let timer = null
    const flush = () => {
      timer = null
      if (
        !currentTrackIdRef.current ||
        playerStateRef.current?.current?.isRadio
      ) {
        return
      }
      const posMs = Math.floor((audioInstance.currentTime || 0) * 1000)
      const state = audioInstance.paused ? 'paused' : 'playing'
      subsonic
        .reportPlayback(currentTrackIdRef.current, posMs, state)
        .catch(() => {})
    }
    const handleSeeked = () => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(flush, 250)
      if (activeOutputDeviceRef.current !== 'local' && audioInstance) {
        const seekTarget = audioInstance.currentTime || 0
        remoteProgressRef.current.baseSec = seekTarget
        remoteProgressRef.current.baseClientTime = Date.now()
        virtualSecRef.current = seekTarget
        httpClient('/api/output/control', {
          method: 'POST',
          body: JSON.stringify({
            channel: 'remote',
            deviceId: activeOutputDeviceRef.current,
            action: 'seek',
            timeOffset: seekTarget >= 1.0 ? seekTarget : 0,
          }),
        }).catch(() => {})
      }
    }
    audioInstance.addEventListener('seeked', handleSeeked)
    return () => {
      if (timer) clearTimeout(timer)
      audioInstance.removeEventListener('seeked', handleSeeked)
    }
  }, [audioInstance])

  const playerContent = (
    <PlayerErrorBoundary>
      <ThemeProvider theme={createMuiTheme(theme)}>
        <ReactJkMusicPlayer
          {...options}
          className={classes.player}
        onAudioListsChange={onAudioListsChange}
        onAudioVolumeChange={onAudioVolumeChange}
        onAudioProgress={onAudioProgress}
        onAudioPlay={onAudioPlay}
        onPlayIndexChange={onPlayIndexChange}
        onAudioPlayTrackChange={onAudioPlayTrackChange}
        onAudioPause={onAudioPause}
        onPlayModeChange={(mode) => {
          dispatch(setPlayMode(mode))
          if (activeOutputDeviceRef.current !== 'local') {
            httpClient('/api/output/control', {
              method: 'POST',
              body: JSON.stringify({
                channel: 'remote',
                deviceId: activeOutputDeviceRef.current,
                action: 'playMode',
                mode,
              }),
            }).catch(() => {})
          }
        }}
        onAudioEnded={onAudioEnded}
        onAudioLyricChange={onAudioLyricChange}
        onCoverClick={onCoverClick}
        onAudioError={onAudioError}
        onBeforeDestroy={onBeforeDestroy}
        onAudioListsPanelChange={(panelVisible) => {
          if (panelVisible) {
            isQueueOpenRef.current = true
            try {
              window.history.pushState({ nd_modal: 'queue' }, '')
            } catch (e) {}

            setTimeout(() => {
              const header = document.querySelector('.audio-lists-panel-header')
              if (header && activeOutputDeviceRef.current !== 'local') {
                let btn = header.querySelector('.nd-import-local-queue-btn')
                if (!btn) {
                  btn = document.createElement('span')
                  btn.className = 'nd-import-local-queue-btn'
                  btn.innerHTML =
                    '<span style="display:inline-flex;align-items:center;gap:4px;cursor:pointer;padding:3px 8px;margin-right:8px;border-radius:12px;background:rgba(25,118,210,0.18);color:#1976d2;font-size:0.75rem;font-weight:600;border:1px solid rgba(25,118,210,0.35);user-select:none" title="将本机列表推送到当前远端输出设备">📥 导入本机列表</span>'
                  btn.onclick = (e) => {
                    e.stopPropagation()
                    handleImportLocalQueueToRemote()
                  }
                  const closeBtn = header.querySelector(
                    '.audio-lists-panel-header-close-btn',
                  )
                  if (closeBtn) {
                    header.insertBefore(btn, closeBtn)
                  } else {
                    header.appendChild(btn)
                  }
                }
              }
            }, 100)
          } else {
            if (isQueueOpenRef.current) {
              isQueueOpenRef.current = false
              if (window.history.state?.nd_modal === 'queue') {
                try {
                  window.history.back()
                } catch (e) {}
              }
            }
          }
        }}
        getAudioInstance={setAudioInstance}
      />
      <GlobalHotKeys handlers={handlers} keyMap={keyMap} allowChanges />
    </ThemeProvider>
    </PlayerErrorBoundary>
  )

  if (typeof document !== 'undefined' && document.body) {
    return ReactDOM.createPortal(playerContent, document.body)
  }
  return playerContent
}

export { Player }

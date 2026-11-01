import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInterval } from '../common'
import { useDispatch, useSelector } from 'react-redux'
import { useMediaQuery } from '@material-ui/core'
import { ThemeProvider } from '@material-ui/core/styles'
import {
  createMuiTheme,
  useAuthState,
  useDataProvider,
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
import { sendNotification } from '../utils'
import subsonic from '../subsonic'
import locale from './locale'
import { keyMap } from '../hotkeys'
import keyHandlers from './keyHandlers'
import { calculateGain } from '../utils/calculateReplayGain'
import { detectBrowserProfile, decisionService } from '../transcode'
import { httpClient } from '../dataProvider'

const Player = () => {
  const theme = useCurrentTheme()
  const translate = useTranslate()
  const playerTheme = theme.player?.theme || 'dark'
  const dataProvider = useDataProvider()
  const playerState = useSelector((state) => state.player)
  const dispatch = useDispatch()
  const [currentTrackId, setCurrentTrackId] = useState(null)
  const [heartbeatTrackId, setHeartbeatTrackId] = useState(null)
  const lastPositionMsRef = useRef(0)
  const currentTrackIdRef = useRef(null)
  const stoppedRef = useRef(false)
  const [audioInstance, setAudioInstance] = useState(null)
  const isDesktop = useMediaQuery('(min-width:810px)')
  const isMobilePlayer =
    /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
      navigator.userAgent,
    )

  const { authenticated } = useAuthState()

  // Keep a ref to playerState so the mount effect can read the latest value
  // without re-triggering on every queue/position change
  const playerStateRef = useRef(playerState)
  playerStateRef.current = playerState

  currentTrackIdRef.current = currentTrackId

  useInterval(
    () => {
      if (heartbeatTrackId && !stoppedRef.current) {
        subsonic.reportPlayback(
          heartbeatTrackId,
          lastPositionMsRef.current,
          'playing',
        )
      }
    },
    heartbeatTrackId ? config.playbackReportIntervalMs : null,
  )

  // Detect browser codec profile and eagerly resolve transcode URLs for the
  // persisted queue once on mount (e.g. after a browser refresh)
  useEffect(() => {
    const profile = detectBrowserProfile()
    decisionService.setProfile(profile)
    dispatch(setTranscodingProfile(profile))

    const state = playerStateRef.current
    const currentIdx = state.savedPlayIndex || 0
    const trackIds = state.queue
      .slice(currentIdx, currentIdx + 4)
      .filter((item) => !item.isRadio && item.trackId)
      .map((item) => item.trackId)

    if (trackIds.length === 0) {
      dispatch(refreshQueue())
      return
    }

    Promise.allSettled(
      trackIds.map((id) =>
        decisionService.resolveStreamUrl(id).then((url) => [id, url]),
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
      .filter((item) => !item.isRadio)
      .map((item) => item.trackId)

    if (nextSongIds.length > 0) {
      decisionService.prefetchDecisions(nextSongIds)
    }
  }, [playerState.queue, playerState.savedPlayIndex])

  const [activeOutputDevice, setActiveOutputDevice] = useState(
    () => localStorage.getItem('nd_active_output_device') || 'local',
  )
  const activeOutputDeviceRef = useRef(activeOutputDevice)
  activeOutputDeviceRef.current = activeOutputDevice

  useEffect(() => {
    httpClient('/api/output/devices?enabled_only=true')
      .then(({ json }) => {
        if (json?.activeId) {
          setActiveOutputDevice(json.activeId)
          localStorage.setItem('nd_active_output_device', json.activeId)
        }
      })
      .catch(() => {})

    const handleDeviceChanged = (e) => {
      const newId = e.detail?.deviceId || 'local'
      setActiveOutputDevice(newId)
    }
    window.addEventListener('nd:output-device-changed', handleDeviceChanged)
    return () => {
      window.removeEventListener('nd:output-device-changed', handleDeviceChanged)
    }
  }, [])

  const isHiddenRef = useRef(false)

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
    document.addEventListener('visibilitychange', handleVisibilityChange)
    window.addEventListener('focus', handleVisibilityChange)
    window.addEventListener('blur', () => {
      isHiddenRef.current = true
    })
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      window.removeEventListener('focus', handleVisibilityChange)
    }
  }, [])

  const virtualSecRef = useRef(0)
  const remoteProgressRef = useRef({
    baseSec: 0,
    baseClientTime: 0,
    playing: false,
    duration: 0,
    trackId: null,
    currentIndex: -1,
  })

  useEffect(() => {
    if (audioInstance) {
      if (activeOutputDevice !== 'local') {
        audioInstance.muted = true

        // Intercept native play/pause so ReactJkMusicPlayer operates virtually without firing hardware DOM audio
        if (!audioInstance._origPlay) {
          audioInstance._origPlay = audioInstance.play.bind(audioInstance)
          audioInstance._origPause = audioInstance.pause.bind(audioInstance)
        }

        audioInstance.play = () => {
          audioInstance.dispatchEvent(new Event('play'))
          return Promise.resolve()
        }

        audioInstance.pause = () => {
          audioInstance.dispatchEvent(new Event('pause'))
        }

        try {
          Object.defineProperty(audioInstance, 'currentTime', {
            get: () => virtualSecRef.current,
            set: (val) => {
              virtualSecRef.current = val
              remoteProgressRef.current.baseSec = val
              remoteProgressRef.current.baseClientTime = Date.now()
            },
            configurable: true,
          })
        } catch (e) {}

        try {
          audioInstance._origPause()
        } catch (e) {}
      } else {
        if (audioInstance._origPlay) {
          audioInstance.play = audioInstance._origPlay
          audioInstance.pause = audioInstance._origPause
        }
        try {
          delete audioInstance.currentTime
        } catch (e) {}
        audioInstance.muted = false
      }
    }
  }, [audioInstance, activeOutputDevice])

  const lastSyncedQueueKeyRef = useRef('')
  useEffect(() => {
    if (activeOutputDevice === 'local' || !playerState.queue.length) return
    const currentIdx = playerState.playIndex ?? playerState.savedPlayIndex ?? 0
    const queueKey = `${playerState.queue.map((t) => t.trackId).join(',')}_${currentIdx}`
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

    httpClient('/api/output/queue', {
      method: 'POST',
      body: JSON.stringify({
        tracks: payloadTracks,
        startIndex: currentIdx,
        timeOffset: (audioInstance?.currentTime && audioInstance.currentTime >= 1.0) ? audioInstance.currentTime : 0,
      }),
    }).catch(() => {})
  }, [activeOutputDevice, playerState.queue, playerState.playIndex, playerState.clear])

  // Sync state from server session every 2 seconds
  useInterval(
    () => {
      if (activeOutputDevice !== 'local') {
        httpClient('/api/output/status')
          .then(({ json }) => {
            if (json) {
              const isServerPlaying = json.state === 'playing'
              remoteProgressRef.current.baseSec = Number(json.timeOffset) || 0
              remoteProgressRef.current.baseClientTime = Date.now()
              remoteProgressRef.current.playing = isServerPlaying
              remoteProgressRef.current.duration = Number(json.track?.duration) || 0
              remoteProgressRef.current.trackId = json.track?.trackId || null
              remoteProgressRef.current.currentIndex = json.currentIndex ?? -1

              // Synchronize play/pause button icon with server state
              if (audioInstance) {
                if (isServerPlaying) {
                  audioInstance.dispatchEvent(new Event('play'))
                } else {
                  audioInstance.dispatchEvent(new Event('pause'))
                }
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
                  } catch (e) {}
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
          } catch (e) {}
        }

        const formatTime = (seconds) => {
          const s = Math.floor(seconds || 0)
          const m = Math.floor(s / 60)
          const sec = s % 60
          return `${m < 10 ? '0' + m : m}:${sec < 10 ? '0' + sec : sec}`
        }
        const timeEls = document.querySelectorAll('.current-time, .music-player-panel .current-time')
        timeEls.forEach((el) => {
          el.innerText = formatTime(currentSec)
        })
        const durEls = document.querySelectorAll('.duration, .music-player-panel .duration')
        if (clock.duration > 0) {
          durEls.forEach((el) => {
            el.innerText = formatTime(clock.duration)
          })
        }
        const trackEls = document.querySelectorAll('.rc-slider-track, .progress-bar .rc-slider-track')
        const handleEls = document.querySelectorAll('.rc-slider-handle, .progress-bar .rc-slider-handle')
        if (clock.duration > 0) {
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
      httpClient('/api/output/control', {
        method: 'POST',
        body: JSON.stringify({ action: 'stop' }),
      }).catch(() => {})
    }
  }, [activeOutputDevice])

  const visible = authenticated
  const placeholderTracks = useMemo(
    () => [
      {
        name: 'Navidrome 就绪',
        singer: '等待播放曲目',
        cover: '/app-icon.png',
        musicSrc: '',
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
  const [context, setContext] = useState(null)
  const [gainNode, setGainNode] = useState(null)

  useEffect(() => {
    if (
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

  const options = useMemo(() => {
    const current = playerState.current || {}
    const hasQueue = playerState.queue.length > 0
    const activePlayIndex =
      playerState.playIndex != null
        ? playerState.playIndex
        : playerState.savedPlayIndex != null && playerState.savedPlayIndex >= 0
        ? playerState.savedPlayIndex
        : 0
    return {
      ...defaultOptions,
      audioLists: hasQueue ? playerState.queue.map((item) => item) : placeholderTracks,
      playIndex: hasQueue ? activePlayIndex : 0,
      autoPlay:
        hasQueue &&
        playerState.autoPlay !== false &&
        (playerState.clear || playerState.playIndex === 0),
      clearPriorAudioLists: playerState.clear,
      extendsContent: (
        <PlayerToolbar id={current.trackId} isRadio={current.isRadio} />
      ),
      defaultVolume: isMobilePlayer ? 1 : playerState.volume,
      showMediaSession: !current.isRadio && hasQueue,
    }
  }, [playerState, defaultOptions, isMobilePlayer, placeholderTracks])

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

          if (remoteProgressRef.current.currentIndex !== playIndex) {
            remoteProgressRef.current.currentIndex = playIndex
            remoteProgressRef.current.trackId = targetTrack.trackId
            remoteProgressRef.current.baseSec = 0
            remoteProgressRef.current.baseClientTime = Date.now()
            remoteProgressRef.current.duration = targetTrack.duration || 0
            remoteProgressRef.current.playing = true
            virtualSecRef.current = 0

            httpClient('/api/output/control', {
              method: 'POST',
              body: JSON.stringify({ action: 'jump', index: playIndex }),
            }).catch(() => {})
          }
        }
      }
    },
    [dispatch],
  )

  const onAudioPlay = useCallback(
    (info) => {
      if (context && context.state !== 'running') {
        context.resume()
      }

      dispatch(currentPlaying(info))
      if (info.isPlaceholder) return

      // Real remote streaming dispatch: keep local audio DOM powered off and send play to server session
      if (activeOutputDeviceRef.current !== 'local') {
        if (audioInstance) {
          audioInstance.muted = true
        }
        const targetIndex = playerStateRef.current?.queue?.findIndex(
          (item) => item.trackId === info.trackId,
        )
        if (
          targetIndex >= 0 &&
          targetIndex !== remoteProgressRef.current.currentIndex
        ) {
          remoteProgressRef.current.currentIndex = targetIndex
          remoteProgressRef.current.trackId = info.trackId
          remoteProgressRef.current.baseSec = 0
          remoteProgressRef.current.baseClientTime = Date.now()
          remoteProgressRef.current.duration = info.duration || 0
          remoteProgressRef.current.playing = true
          virtualSecRef.current = 0
          httpClient('/api/output/control', {
            method: 'POST',
            body: JSON.stringify({ action: 'jump', index: targetIndex }),
          }).catch(() => {})
        } else {
          if (!remoteProgressRef.current.playing) {
            remoteProgressRef.current.playing = true
            remoteProgressRef.current.baseClientTime = Date.now()
            httpClient('/api/output/control', {
              method: 'POST',
              body: JSON.stringify({ action: 'play' }),
            }).catch(() => {})
          }
        }
      }

      if (info.duration) {
        const song = info.song
        document.title = `${song.title} - ${song.artist} - Navidrome`
        if (!info.isRadio) {
          const posMs = Math.floor(info.currentTime * 1000)
          lastPositionMsRef.current = posMs
          const isNewTrack = info.trackId !== currentTrackId
          if (isNewTrack) {
            subsonic
              .reportPlayback(info.trackId, posMs, 'starting')
              .then(() =>
                subsonic.reportPlayback(info.trackId, posMs, 'playing'),
              )
            setCurrentTrackId(info.trackId)
          } else {
            subsonic.reportPlayback(info.trackId, posMs, 'playing')
          }
          setHeartbeatTrackId(info.trackId)
        }
        if (config.gaTrackingId) {
          ReactGA.event({
            category: 'Player',
            action: 'Play song',
            label: `${song.title} - ${song.artist}`,
          })
        }
        if (showNotifications) {
          sendNotification(
            song.title,
            `${song.artist} - ${song.album}`,
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
        subsonic.reportPlayback(
          currentTrackId,
          lastPositionMsRef.current,
          'stopped',
        )
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
    }
  }, [currentTrackId])

  const onAudioPause = useCallback(
    (info) => {
      dispatch(currentPlaying(info))
      // When remote output is active:
      // STRICT GUARD: If tab is hidden/minimized/in background, or screen is sleeping,
      // the pause event is a synthetic browser power-saving throttling event, NOT a user intent!
      // Do NOT send pause to remote hardware in background!
      if (activeOutputDeviceRef.current !== 'local' && !info.isPlaceholder) {
        const elapsed = remoteProgressRef.current.baseClientTime > 0
          ? (Date.now() - remoteProgressRef.current.baseClientTime) / 1000
          : 0
        remoteProgressRef.current.baseSec += Math.max(0, elapsed)
        remoteProgressRef.current.baseClientTime = Date.now()
        remoteProgressRef.current.playing = false
        if (!document.hidden && !isHiddenRef.current) {
          httpClient('/api/output/control', {
            method: 'POST',
            body: JSON.stringify({ action: 'pause' }),
          }).catch(() => {})
        }
      }
      if (!info.isRadio && currentTrackId) {
        const posMs = Math.floor(info.currentTime * 1000)
        lastPositionMsRef.current = posMs
        subsonic.reportPlayback(currentTrackId, posMs, 'paused')
      }
      setHeartbeatTrackId(null)
    },
    [dispatch, currentTrackId],
  )

  const onAudioEnded = useCallback(
    (currentPlayId, audioLists, info) => {
      if (info.isPlaceholder) return
      if (activeOutputDeviceRef.current !== 'local') return
      if (currentTrackId && !info.isRadio) {
        const posMs = Math.floor((info.duration || 0) * 1000)
        subsonic.reportPlayback(currentTrackId, posMs, 'stopped')
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(currentPlaying(info))
      dataProvider
        .getOne('keepalive', { id: info.trackId })
        // eslint-disable-next-line no-console
        .catch((e) => console.log('Keepalive error:', e))
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
          .filter((item) => !item.isRadio)
          .map((item) => item.trackId)
        if (nextSongIds.length > 0) {
          decisionService.prefetchDecisions(nextSongIds)
        }
      }
    },
    [playerState.queue],
  )

  const onBeforeDestroy = useCallback(() => {
    return new Promise((resolve, reject) => {
      if (currentTrackId && !playerStateRef.current?.current?.isRadio) {
        subsonic.reportPlayback(
          currentTrackId,
          lastPositionMsRef.current,
          'stopped',
        )
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      dispatch(clearQueue())
      reject()
    })
  }, [dispatch, currentTrackId])

  if (!visible) {
    document.title = 'Navidrome'
  }

  const handlers = useMemo(
    () => keyHandlers(audioInstance, playerState),
    [audioInstance, playerState],
  )

  useEffect(() => {
    if (isMobilePlayer && audioInstance) {
      audioInstance.volume = 1
    }
  }, [isMobilePlayer, audioInstance])

  // MediaSession position state & mobile background resume breakpoint recovery
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

    // Recover playback position on mobile Webview/tab resume if stream stalled or reset to 0
    const handleVisibilityOrResume = () => {
      if (document.visibilityState === 'visible' && audioInstance) {
        const savedPosSec = (lastPositionMsRef.current || 0) / 1000
        if (
          savedPosSec > 2 &&
          (audioInstance.currentTime === 0 || audioInstance.currentTime == null) &&
          !audioInstance.ended &&
          currentTrackIdRef.current
        ) {
          try {
            audioInstance.currentTime = savedPosSec
            if (!audioInstance.paused) {
              audioInstance.play().catch(() => {})
            }
          } catch (e) {
            // Seek failed or media loading
          }
        }
      }
    }

    audioInstance.addEventListener('timeupdate', updatePositionState)
    audioInstance.addEventListener('durationchange', updatePositionState)
    document.addEventListener('visibilitychange', handleVisibilityOrResume)
    window.addEventListener('online', handleVisibilityOrResume)

    return () => {
      audioInstance.removeEventListener('timeupdate', updatePositionState)
      audioInstance.removeEventListener('durationchange', updatePositionState)
      document.removeEventListener('visibilitychange', handleVisibilityOrResume)
      window.removeEventListener('online', handleVisibilityOrResume)
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
      subsonic.reportPlayback(currentTrackIdRef.current, posMs, state)
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

  return (
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
        onPlayModeChange={(mode) => dispatch(setPlayMode(mode))}
        onAudioEnded={onAudioEnded}
        onCoverClick={onCoverClick}
        onAudioError={onAudioError}
        onBeforeDestroy={onBeforeDestroy}
        getAudioInstance={setAudioInstance}
      />
      <GlobalHotKeys handlers={handlers} keyMap={keyMap} allowChanges />
    </ThemeProvider>
  )
}

export { Player }

import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch } from 'react-redux'
import {
  Card,
  CardContent,
  Typography,
  makeStyles,
  Button,
  IconButton,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Box,
  CircularProgress,
} from '@material-ui/core'
import HistoryIcon from '@material-ui/icons/History'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistPlayIcon from '@material-ui/icons/PlaylistPlay'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import DeleteOutlineIcon from '@material-ui/icons/DeleteOutline'
import MoreVertIcon from '@material-ui/icons/MoreVert'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import {
  playTracks,
  playNext,
  addTracks,
  openAddToPlaylist,
} from '../actions'
import { ensureShadowTrack } from '../onlinemusic/api'
import { QualityBadge, SourceBadge } from '../onlinemusic/QualityBadge'
import { sanitizeImageUrl } from '../utils'

const STORAGE_KEY = 'nd_recently_played_records'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(1.5),
    [theme.breakpoints.up('md')]: {
      padding: theme.spacing(2, 3),
    },
    width: '100%',
    maxWidth: '100vw',
    margin: 0,
    boxSizing: 'border-box',
    overflowX: 'hidden',
    paddingBottom: 120,
  },
  card: {
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
    overflowX: 'hidden',
    borderRadius: 12,
  },
  cardContent: {
    padding: theme.spacing(1.5),
    [theme.breakpoints.up('sm')]: {
      padding: theme.spacing(2),
    },
    boxSizing: 'border-box',
    overflowX: 'hidden',
  },
  topBar: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    flexWrap: 'wrap',
    gap: theme.spacing(1),
    marginBottom: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    paddingBottom: theme.spacing(1.5),
  },
  titleGroup: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  },
  headerActions: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  trackList: {
    padding: 0,
    width: '100%',
    boxSizing: 'border-box',
  },
  trackItem: {
    display: 'flex',
    alignItems: 'center',
    padding: '10px 8px',
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
    borderRadius: 8,
    borderBottom: `1px solid ${theme.palette.divider}`,
    transition: 'background-color 0.15s ease',
    cursor: 'pointer',
    overflow: 'hidden',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  indexCol: {
    width: 28,
    minWidth: 28,
    textAlign: 'center',
    color: theme.palette.text.secondary,
    fontSize: '0.85rem',
    marginRight: 8,
    flexShrink: 0,
  },
  coverContainer: {
    width: 44,
    height: 44,
    minWidth: 44,
    borderRadius: 6,
    overflow: 'hidden',
    marginRight: 12,
    backgroundColor: theme.palette.background.default,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flexShrink: 0,
  },
  coverImg: {
    width: '100%',
    height: '100%',
    objectFit: 'cover',
  },
  trackMain: {
    flex: 1,
    minWidth: 0,
    overflow: 'hidden',
    marginRight: 8,
  },
  trackTitle: {
    fontWeight: 600,
    fontSize: '0.92rem',
    color: theme.palette.text.primary,
    marginBottom: 3,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: '100%',
  },
  trackMeta: {
    display: 'flex',
    alignItems: 'center',
    fontSize: '0.78rem',
    color: theme.palette.text.secondary,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
    maxWidth: '100%',
    gap: 4,
  },
  artistText: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
    flexShrink: 1,
  },
  localBadge: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '1px 5px',
    borderRadius: '3px',
    fontSize: '0.65rem',
    fontWeight: 700,
    letterSpacing: '0.3px',
    userSelect: 'none',
    lineHeight: 1.2,
    marginRight: '6px',
    background: 'rgba(16, 185, 129, 0.12)',
    color: '#10B981',
    border: '1px solid rgba(16, 185, 129, 0.3)',
  },
  timeCol: {
    color: theme.palette.text.secondary,
    fontSize: '0.75rem',
    width: 64,
    minWidth: 64,
    textAlign: 'right',
    marginRight: 8,
    flexShrink: 0,
    '@media (max-width: 600px)': {
      display: 'none',
    },
  },
  durationCol: {
    color: theme.palette.text.secondary,
    fontSize: '0.8rem',
    width: 44,
    minWidth: 44,
    textAlign: 'right',
    marginRight: 4,
    flexShrink: 0,
  },
  actionButton: {
    padding: 4,
    flexShrink: 0,
  },
  emptyContainer: {
    textAlign: 'center',
    padding: theme.spacing(8, 2),
    color: theme.palette.text.secondary,
  },
}))

export const RecentlyPlayed = () => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const [records, setRecords] = useState([])
  const [menuAnchorEl, setMenuAnchorEl] = useState(null)
  const [activeMenuTrack, setActiveMenuTrack] = useState(null)
  const [actingId, setActingId] = useState(null)

  const loadRecords = useCallback(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (raw) {
        const parsed = JSON.parse(raw)
        if (Array.isArray(parsed)) {
          setRecords(parsed)
          return
        }
      }
    } catch (e) {}
    setRecords([])
  }, [])

  useEffect(() => {
    loadRecords()

    const handleUpdated = () => {
      loadRecords()
    }
    window.addEventListener('nd:recently-played-updated', handleUpdated)
    window.addEventListener('storage', handleUpdated)
    return () => {
      window.removeEventListener('nd:recently-played-updated', handleUpdated)
      window.removeEventListener('storage', handleUpdated)
    }
  }, [loadRecords])

  const formatRelativeTime = (timestamp) => {
    if (!timestamp) return ''
    const diffSec = Math.floor((Date.now() - timestamp) / 1000)
    if (diffSec < 60) return '刚刚'
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}分钟前`
    const diffHour = Math.floor(diffMin / 60)
    if (diffHour < 24) return `${diffHour}小时前`
    const diffDay = Math.floor(diffHour / 24)
    if (diffDay < 7) return `${diffDay}天前`
    const date = new Date(timestamp)
    return `${date.getMonth() + 1}-${date.getDate()}`
  }

  const formatDuration = (sec) => {
    if (!sec || isNaN(sec)) return '--:--'
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m < 10 ? '0' : ''}${m}:${s < 10 ? '0' : ''}${s}`
  }

  const buildClientMediaFile = (track) => {
    const isOnline = track.source && track.source !== 'local'
    const q = track.quality || 'flac'
    return {
      id: track.id,
      title: track.title,
      artist: track.artist || '未知歌手',
      album: track.album || '',
      duration: track.duration || 240,
      path:
        track.path ||
        (isOnline
          ? `online://${track.source}/${track.songId || track.id}/${q}`
          : ''),
      cover: sanitizeImageUrl(track.cover || ''),
      coverUrl: sanitizeImageUrl(track.cover || ''),
      hasCoverArt: !!track.cover,
    }
  }

  const handlePlay = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(playTracks({ [mf.id]: mf }, [mf.id], mf.id))
    if (track.source && track.source !== 'local') {
      ensureShadowTrack(track).catch(() => {})
    }
  }

  const handlePlayNext = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(playNext({ [mf.id]: mf }, [mf.id]))
    if (track.source && track.source !== 'local') {
      ensureShadowTrack(track).catch(() => {})
    }
  }

  const handleAddToQueue = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(addTracks({ [mf.id]: mf }, [mf.id]))
    if (track.source && track.source !== 'local') {
      ensureShadowTrack(track).catch(() => {})
    }
  }

  const handleAddToPlaylist = async (track, e) => {
    if (e) e.stopPropagation()
    setActingId(track.id)
    try {
      if (track.source && track.source !== 'local') {
        const res = await ensureShadowTrack(track)
        if (res && res.mediaFile) {
          dispatch(openAddToPlaylist({ selectedIds: [res.mediaFile.id] }))
        }
      } else {
        dispatch(openAddToPlaylist({ selectedIds: [track.id] }))
      }
    } catch (err) {
      console.error('Failed to add to playlist:', err)
    } finally {
      setActingId(null)
    }
  }

  const handlePlayAll = () => {
    if (!records.length) return
    const data = {}
    const ids = []
    records.forEach((track) => {
      const mf = buildClientMediaFile(track)
      data[mf.id] = mf
      ids.push(mf.id)
    })
    dispatch(playTracks(data, ids, ids[0]))
    records
      .filter((t) => t.source && t.source !== 'local')
      .slice(0, 10)
      .forEach((t) => {
        ensureShadowTrack(t).catch(() => {})
      })
  }

  const handleRemoveTrack = (trackId, e) => {
    if (e) e.stopPropagation()
    const next = records.filter((t) => t.id !== trackId)
    setRecords(next)
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
    } catch (err) {}
  }

  const handleClearAll = () => {
    if (window.confirm('确定要清空全部最近播放记录吗？')) {
      setRecords([])
      try {
        localStorage.removeItem(STORAGE_KEY)
      } catch (err) {}
    }
  }

  return (
    <div className={classes.root}>
      <Card className={classes.card}>
        <CardContent className={classes.cardContent}>
          <div className={classes.topBar}>
            <div className={classes.titleGroup}>
              <HistoryIcon style={{ fontSize: 28, color: '#3B82F6' }} />
              <div>
                <Typography variant="h6" style={{ fontWeight: 700, lineHeight: 1.2 }}>
                  最近播放
                </Typography>
                <Typography variant="caption" color="textSecondary">
                  共 {records.length} 首播放记录
                </Typography>
              </div>
            </div>

            <div className={classes.headerActions}>
              <Button
                variant="contained"
                color="primary"
                size="small"
                startIcon={<PlayArrowIcon />}
                onClick={handlePlayAll}
                disabled={records.length === 0}
              >
                全部播放
              </Button>
              <Button
                variant="outlined"
                size="small"
                startIcon={<DeleteOutlineIcon />}
                onClick={handleClearAll}
                disabled={records.length === 0}
              >
                清空记录
              </Button>
            </div>
          </div>

          {records.length === 0 ? (
            <div className={classes.emptyContainer}>
              <HistoryIcon style={{ fontSize: 48, color: 'rgba(156, 163, 175, 0.4)', marginBottom: 12 }} />
              <Typography variant="body1">暂无最近播放记录</Typography>
              <Typography variant="body2" color="textSecondary" style={{ marginTop: 4 }}>
                播放曲库歌曲或网络音乐后将自动收录在此
              </Typography>
            </div>
          ) : (
            <div className={classes.trackList}>
              {records.map((track, idx) => (
                <div
                  key={track.id || idx}
                  className={classes.trackItem}
                  onClick={(e) => {
                    e.stopPropagation()
                    setMenuAnchorEl(e.currentTarget)
                    setActiveMenuTrack(track)
                  }}
                >
                  <div className={classes.indexCol}>{idx + 1}</div>

                  <div className={classes.coverContainer}>
                    {track.cover ? (
                      <img
                        className={classes.coverImg}
                        src={sanitizeImageUrl(track.cover)}
                        alt={track.title}
                        onError={(e) => {
                          e.currentTarget.style.display = 'none'
                        }}
                      />
                    ) : (
                      <MusicNoteIcon color="disabled" fontSize="small" />
                    )}
                  </div>

                  <div className={classes.trackMain}>
                    <div className={classes.trackTitle}>{track.title}</div>
                    <div className={classes.trackMeta}>
                      {track.source && track.source !== 'local' ? (
                        <>
                          <QualityBadge quality={track.quality || 'flac'} />
                          <SourceBadge source={track.source} />
                        </>
                      ) : (
                        <span className={classes.localBadge}>本地曲库</span>
                      )}
                      <span className={classes.artistText}>
                        {track.artist || '未知歌手'}
                        {track.album ? ` - ${track.album}` : ''}
                      </span>
                    </div>
                  </div>

                  <div className={classes.timeCol}>
                    {formatRelativeTime(track.playedAt)}
                  </div>

                  <div className={classes.durationCol}>
                    {formatDuration(track.duration)}
                  </div>

                  <IconButton
                    size="small"
                    className={classes.actionButton}
                    onClick={(e) => {
                      e.stopPropagation()
                      setMenuAnchorEl(e.currentTarget)
                      setActiveMenuTrack(track)
                    }}
                  >
                    {actingId === track.id ? (
                      <CircularProgress size={18} />
                    ) : (
                      <MoreVertIcon fontSize="small" />
                    )}
                  </IconButton>
                </div>
              ))}
            </div>
          )}

          {/* Action Menu (⋮) */}
          <Menu
            anchorEl={menuAnchorEl}
            open={Boolean(menuAnchorEl)}
            onClose={() => {
              setMenuAnchorEl(null)
              setActiveMenuTrack(null)
            }}
          >
            <MenuItem
              onClick={(e) => {
                if (activeMenuTrack) handlePlay(activeMenuTrack, e)
                setMenuAnchorEl(null)
              }}
            >
              <ListItemIcon>
                <PlayArrowIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="立即播放" />
            </MenuItem>
            <MenuItem
              onClick={(e) => {
                if (activeMenuTrack) handlePlayNext(activeMenuTrack, e)
                setMenuAnchorEl(null)
              }}
            >
              <ListItemIcon>
                <PlaylistPlayIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="下一首播放" />
            </MenuItem>
            <MenuItem
              onClick={(e) => {
                if (activeMenuTrack) handleAddToQueue(activeMenuTrack, e)
                setMenuAnchorEl(null)
              }}
            >
              <ListItemIcon>
                <QueueMusicIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="添加到播放队列" />
            </MenuItem>
            <MenuItem
              onClick={(e) => {
                if (activeMenuTrack) handleAddToPlaylist(activeMenuTrack, e)
                setMenuAnchorEl(null)
              }}
            >
              <ListItemIcon>
                <PlaylistAddIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="添加到歌单" />
            </MenuItem>
            <MenuItem
              onClick={(e) => {
                if (activeMenuTrack) handleRemoveTrack(activeMenuTrack.id, e)
                setMenuAnchorEl(null)
              }}
            >
              <ListItemIcon>
                <DeleteOutlineIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="从记录中移除" />
            </MenuItem>
          </Menu>
        </CardContent>
      </Card>
    </div>
  )
}

export default RecentlyPlayed

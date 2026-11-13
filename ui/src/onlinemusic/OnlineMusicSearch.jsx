import React, { useState, useEffect, useCallback, useRef } from 'react'
import { useDispatch } from 'react-redux'
import {
  Card,
  CardContent,
  Typography,
  TextField,
  Tabs,
  Tab,
  IconButton,
  Tooltip,
  CircularProgress,
  makeStyles,
  Box,
  Button,
  Menu,
  MenuItem,
  Paper,
  ClickAwayListener,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  Divider,
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistPlayIcon from '@material-ui/icons/PlaylistPlay'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import SearchIcon from '@material-ui/icons/Search'
import HistoryIcon from '@material-ui/icons/History'
import ClearIcon from '@material-ui/icons/Clear'
import MoreVertIcon from '@material-ui/icons/MoreVert'
import ArrowDropDownIcon from '@material-ui/icons/ArrowDropDown'
import { playTracks, playNext, addTracks, openAddToPlaylist } from '../actions'
import { sanitizeImageUrl } from '../utils'
import { searchOnline, ensureShadowTrack, getSearchTips } from './api'
import { QualityBadge, SourceBadge } from './QualityBadge'

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
    paddingBottom: 110,
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
    marginBottom: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    paddingBottom: theme.spacing(1),
  },
  searchRow: {
    display: 'flex',
    alignItems: 'center',
    position: 'relative',
    marginBottom: theme.spacing(2),
    gap: theme.spacing(1),
    width: '100%',
    boxSizing: 'border-box',
  },
  sourceSelectBtn: {
    textTransform: 'none',
    fontWeight: 600,
    minWidth: 104,
    whiteSpace: 'nowrap',
    fontSize: '0.85rem',
    height: 44,
    padding: '0 8px',
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    flexShrink: 0,
    '& .MuiButton-label': {
      whiteSpace: 'nowrap',
      display: 'inline-flex',
      alignItems: 'center',
    },
  },
  searchBoxContainer: {
    position: 'relative',
    flex: 1,
    minWidth: 0,
  },
  searchInput: {
    '& .MuiOutlinedInput-root': {
      borderRadius: 8,
      height: 44,
    },
  },
  tipsPaper: {
    position: 'absolute',
    top: 52,
    left: 0,
    right: 0,
    zIndex: 1200,
    maxHeight: 280,
    overflowY: 'auto',
    boxShadow: '0 8px 24px rgba(0, 0, 0, 0.15)',
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
  },
  tipItem: {
    paddingTop: 8,
    paddingBottom: 8,
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  trackList: {
    padding: 0,
    width: '100%',
    boxSizing: 'border-box',
  },
  trackItem: {
    display: 'flex',
    alignItems: 'center',
    padding: '10px 6px',
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
    borderRadius: 6,
    borderBottom: `1px solid ${theme.palette.divider}`,
    transition: 'background-color 0.15s ease',
    cursor: 'pointer',
    overflow: 'hidden',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  indexCol: {
    width: 26,
    minWidth: 26,
    textAlign: 'center',
    color: theme.palette.text.secondary,
    fontSize: '0.8rem',
    marginRight: 6,
    flexShrink: 0,
  },
  trackMain: {
    flex: 1,
    minWidth: 0,
    overflow: 'hidden',
    marginRight: 6,
  },
  trackTitle: {
    fontWeight: 600,
    fontSize: '0.9rem',
    color: theme.palette.text.primary,
    marginBottom: 2,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: '100%',
  },
  trackMeta: {
    display: 'flex',
    alignItems: 'center',
    fontSize: '0.75rem',
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
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    padding: theme.spacing(8),
  },
  emptyContainer: {
    textAlign: 'center',
    padding: theme.spacing(8),
    color: theme.palette.text.secondary,
  },
}))

const SOURCE_OPTIONS = [
  { id: 'all', label: '🌐 聚合大会' },
  { id: 'wy', label: '☁️ 小芸音乐' },
  { id: 'kw', label: '🐌 小蜗音乐' },
  { id: 'kg', label: '🐕 小构音乐' },
  { id: 'tx', label: '🐧 小秋音乐' },
  { id: 'mg', label: '🍯 小蜜音乐' },
]

const SEARCH_HISTORY_KEY = 'nd_online_search_history'
const MAX_SEARCH_HISTORY = 10

const loadSearchHistory = () => {
  try {
    const raw = localStorage.getItem(SEARCH_HISTORY_KEY)
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed.slice(0, MAX_SEARCH_HISTORY) : []
  } catch {
    return []
  }
}

const persistSearchHistory = (list) => {
  try {
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(list))
  } catch {
    // ignore storage quota errors
  }
}

export const OnlineMusicSearch = () => {
  const classes = useStyles()
  const dispatch = useDispatch()

  const [source, setSource] = useState('all')
  const [keyword, setKeyword] = useState('')
  const [tabType, setTabType] = useState('songs')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState([])
  const [actingId, setActingId] = useState(null)

  // Source selection menu
  const [sourceAnchorEl, setSourceAnchorEl] = useState(null)

  // Tips autocomplete & search history
  const [tips, setTips] = useState([])
  const [showTips, setShowTips] = useState(false)
  const [searchHistory, setSearchHistory] = useState(() => loadSearchHistory())

  // Track action menu
  const [menuAnchorEl, setMenuAnchorEl] = useState(null)
  const [activeMenuTrack, setActiveMenuTrack] = useState(null)

  const searchBoxRef = useRef(null)
  const skipTipsRef = useRef(false)

  const recordSearchHistory = useCallback((term) => {
    const trimmed = (term || '').trim()
    if (!trimmed) return
    setSearchHistory((prev) => {
      const next = [trimmed, ...prev.filter((item) => item !== trimmed)].slice(
        0,
        MAX_SEARCH_HISTORY,
      )
      persistSearchHistory(next)
      return next
    })
  }, [])

  const clearSearchHistory = useCallback(() => {
    setSearchHistory([])
    try {
      localStorage.removeItem(SEARCH_HISTORY_KEY)
    } catch {
      // ignore
    }
    setShowTips(false)
  }, [])

  const doSearch = useCallback(
    async (src, query) => {
      const trimmed = (query || '').trim()
      if (!trimmed) {
        setResults([])
        return
      }
      recordSearchHistory(trimmed)
      setLoading(true)
      try {
        const res = await searchOnline(src, trimmed, 1, 30)
        setResults(res.list || [])
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('Search error:', err)
        setResults([])
      } finally {
        setLoading(false)
      }
    },
    [recordSearchHistory],
  )

  // 150ms debounced search tips
  useEffect(() => {
    if (skipTipsRef.current) {
      skipTipsRef.current = false
      return
    }
    if (!keyword || keyword.trim() === '') {
      setTips([])
      return
    }

    const timer = setTimeout(async () => {
      try {
        const res = await getSearchTips(keyword.trim())
        if (res && res.tips && res.tips.length > 0) {
          setTips(res.tips)
          setShowTips(true)
        } else {
          setTips([])
          setShowTips(false)
        }
      } catch (e) {
        setTips([])
      }
    }, 150)

    return () => clearTimeout(timer)
  }, [keyword])

  const handleSelectTip = (tip) => {
    skipTipsRef.current = true
    setKeyword(tip)
    setTips([])
    setShowTips(false)
    doSearch(source, tip)
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      skipTipsRef.current = true
      setShowTips(false)
      if (keyword.trim()) {
        doSearch(source, keyword)
      }
    }
  }

  const handleSelectSource = (sId) => {
    setSource(sId)
    setSourceAnchorEl(null)
    if (keyword.trim()) {
      doSearch(sId, keyword)
    }
  }

  const formatDuration = (sec) => {
    if (!sec || isNaN(sec)) return '--:--'
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m < 10 ? '0' : ''}${m}:${s < 10 ? '0' : ''}${s}`
  }

  const buildClientMediaFile = (track) => {
    const q = track.quality || 'flac'
    const coverUrl = sanitizeImageUrl(track.cover || track.coverUrl || '')
    return {
      id: track.id,
      title: track.title,
      artist: track.artist || '未知歌手',
      album: track.album || '',
      duration: track.duration || 240,
      path: `online://${track.source}/${track.songId}/${q}`,
      cover: coverUrl,
      coverUrl,
      hasCoverArt: !!coverUrl,
      lyrics: track.lyrics || track.lyric || '',
    }
  }

  const handlePlay = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    // Synchronously dispatch within user gesture to guarantee mobile autoplay
    dispatch(playTracks({ [mf.id]: mf }, [mf.id], mf.id))
    // Sync to backend shadow track without blocking audio playback
    ensureShadowTrack(track).catch(() => {})
  }

  const handlePlayNext = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(playNext({ [mf.id]: mf }, [mf.id]))
    ensureShadowTrack(track).catch(() => {})
  }

  const handleAddToQueue = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(addTracks({ [mf.id]: mf }, [mf.id]))
    ensureShadowTrack(track).catch(() => {})
  }

  const handleAddToPlaylist = async (track, e) => {
    if (e) e.stopPropagation()
    setActingId(track.id)
    try {
      const res = await ensureShadowTrack(track)
      if (res && res.mediaFile) {
        dispatch(openAddToPlaylist({ selectedIds: [res.mediaFile.id] }))
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to add to playlist:', err)
    } finally {
      setActingId(null)
    }
  }

  const currentSourceLabel =
    SOURCE_OPTIONS.find((s) => s.id === source)?.label || '聚合大会'

  return (
    <div className={classes.root}>
      <Card className={classes.card}>
        <CardContent className={classes.cardContent}>
          <div className={classes.topBar}>
            <Box display="flex" alignItems="center">
              <SearchIcon style={{ marginRight: 8, color: '#3B82F6' }} />
              <Typography variant="h6">音乐搜索</Typography>
            </Box>

            <Tabs
              value={tabType}
              onChange={(e, val) => setTabType(val)}
              indicatorColor="primary"
              textColor="primary"
            >
              <Tab value="songs" label="歌曲" />
              <Tab value="playlists" label="歌单" />
            </Tabs>
          </div>

          <div className={classes.searchRow}>
            {/* Category Dropdown (default: 聚合大会) */}
            <Button
              className={classes.sourceSelectBtn}
              variant="outlined"
              onClick={(e) => setSourceAnchorEl(e.currentTarget)}
              endIcon={<ArrowDropDownIcon />}
            >
              {currentSourceLabel}
            </Button>
            <Menu
              anchorEl={sourceAnchorEl}
              open={Boolean(sourceAnchorEl)}
              onClose={() => setSourceAnchorEl(null)}
            >
              {SOURCE_OPTIONS.map((opt) => (
                <MenuItem
                  key={opt.id}
                  selected={opt.id === source}
                  onClick={() => handleSelectSource(opt.id)}
                  style={{
                    fontWeight: opt.id === source ? 700 : 400,
                    color: opt.id === source ? '#3B82F6' : 'inherit',
                  }}
                >
                  {opt.label}
                </MenuItem>
              ))}
            </Menu>

            {/* Input with real-time tips */}
            <div className={classes.searchBoxContainer} ref={searchBoxRef}>
              <TextField
                className={classes.searchInput}
                fullWidth
                variant="outlined"
                placeholder="搜索歌曲、歌手、专辑..."
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                onKeyDown={handleKeyDown}
                onFocus={() => {
                  if (
                    (keyword.trim() === '' && searchHistory.length > 0) ||
                    (keyword.trim() !== '' && tips.length > 0)
                  ) {
                    setShowTips(true)
                  }
                }}
                InputProps={{
                  endAdornment: keyword ? (
                    <IconButton
                      size="small"
                      onClick={() => {
                        setKeyword('')
                        setTips([])
                        setShowTips(searchHistory.length > 0)
                      }}
                    >
                      <ClearIcon fontSize="small" />
                    </IconButton>
                  ) : (
                    <IconButton
                      size="small"
                      onClick={() => {
                        setShowTips(false)
                        doSearch(source, keyword)
                      }}
                    >
                      <SearchIcon fontSize="small" />
                    </IconButton>
                  ),
                }}
              />

              {showTips &&
                (keyword.trim() === ''
                  ? searchHistory.length > 0
                  : tips.length > 0) && (
                  <ClickAwayListener onClickAway={() => setShowTips(false)}>
                    <Paper className={classes.tipsPaper}>
                      {keyword.trim() === '' ? (
                        <List dense>
                          <Box
                            display="flex"
                            alignItems="center"
                            justifyContent="space-between"
                            px={2}
                            py={0.75}
                          >
                            <Typography variant="caption" color="textSecondary">
                              最近搜索
                            </Typography>
                            <Button
                              size="small"
                              onClick={clearSearchHistory}
                              style={{
                                minWidth: 'auto',
                                padding: '0 6px',
                                fontSize: '0.75rem',
                              }}
                            >
                              清空
                            </Button>
                          </Box>
                          <Divider />
                          {searchHistory.map((item, idx) => (
                            <ListItem
                              key={`hist-${idx}`}
                              className={classes.tipItem}
                              button
                              onClick={() => handleSelectTip(item)}
                            >
                              <ListItemIcon style={{ minWidth: 32 }}>
                                <HistoryIcon fontSize="small" color="disabled" />
                              </ListItemIcon>
                              <ListItemText primary={item} />
                            </ListItem>
                          ))}
                        </List>
                      ) : (
                        <List dense>
                          {tips.map((tip, idx) => (
                            <ListItem
                              key={idx}
                              className={classes.tipItem}
                              button
                              onClick={() => handleSelectTip(tip)}
                            >
                              <ListItemIcon style={{ minWidth: 32 }}>
                                <SearchIcon fontSize="small" color="disabled" />
                              </ListItemIcon>
                              <ListItemText primary={tip} />
                            </ListItem>
                          ))}
                        </List>
                      )}
                    </Paper>
                  </ClickAwayListener>
                )}
            </div>
          </div>

          {/* Results List */}
          {loading ? (
            <div className={classes.loadingContainer}>
              <CircularProgress size={36} />
            </div>
          ) : results.length === 0 ? (
            <div className={classes.emptyContainer}>
              <Typography variant="body1">
                {keyword ? '未搜索到相关曲目，请尝试更换关键词或搜索源' : '输入关键词，回车或点击联想词即刻开搜'}
              </Typography>
            </div>
          ) : (
            <div className={classes.trackList}>
              {results.map((track, idx) => (
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

                  <div className={classes.trackMain}>
                    <div className={classes.trackTitle}>{track.title}</div>
                    <div className={classes.trackMeta}>
                      <QualityBadge quality={track.quality || 'flac'} />
                      <SourceBadge source={track.source} />
                      <span className={classes.artistText}>
                        {track.artist || '未知歌手'}
                        {track.album ? ` - ${track.album}` : ''}
                      </span>
                    </div>
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
              <ListItemText primary="添加到下一首" />
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
              <ListItemText primary="收藏到我的歌单" />
            </MenuItem>
          </Menu>
        </CardContent>
      </Card>
    </div>
  )
}

export default OnlineMusicSearch

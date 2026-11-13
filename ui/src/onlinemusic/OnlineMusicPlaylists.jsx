import React, { useState, useEffect } from 'react'
import { useDispatch } from 'react-redux'
import {
  Card,
  CardContent,
  Typography,
  Grid,
  CardActionArea,
  CardMedia,
  makeStyles,
  Tabs,
  Tab,
  Box,
  CircularProgress,
  Button,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  Paper,
} from '@material-ui/core'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import { playTracks, addTracks, openAddToPlaylist } from '../actions'
import { sanitizeImageUrl } from '../utils'
import { getSonglists, getSonglistDetail, ensureShadowTrack } from './api'
import { QualityBadge, SourceBadge } from './QualityBadge'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(2, 2.5),
    width: '100%',
    maxWidth: '100vw',
    overflowX: 'hidden',
    margin: 0,
    boxSizing: 'border-box',
    paddingBottom: 110,
    '@media (max-width: 768px)': {
      padding: '12px 10px 110px 10px !important',
    },
  },
  card: {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    transition: 'transform 0.2s, box-shadow 0.2s',
    borderRadius: 8,
    '&:hover': {
      transform: 'translateY(-4px)',
      boxShadow: '0 8px 24px rgba(0, 0, 0, 0.2)',
    },
  },
  media: {
    height: 180,
    backgroundColor: theme.palette.grey[900],
  },
  title: {
    fontWeight: 600,
    fontSize: '0.95rem',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    display: '-webkit-box',
    '-webkit-line-clamp': 2,
    '-webkit-box-orient': 'vertical',
  },
  author: {
    color: theme.palette.text.secondary,
    fontSize: '0.8rem',
    marginTop: theme.spacing(0.5),
  },
  sourceTabs: {
    marginBottom: theme.spacing(3),
    borderBottom: `1px solid ${theme.palette.divider}`,
    [theme.breakpoints.down('xs')]: {
      display: 'none',
    },
  },
  sourceGridMobile: {
    display: 'none',
    marginBottom: theme.spacing(2.5),
    [theme.breakpoints.down('xs')]: {
      display: 'flex',
    },
  },
  sourceCapsule: {
    borderRadius: 8,
    textTransform: 'none',
    fontWeight: 600,
    fontSize: '0.85rem',
    height: 38,
    boxShadow: 'none',
  },
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    padding: theme.spacing(8),
  },
  // Full-page detail view styles
  detailPage: {
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
  },
  backBtn: {
    marginBottom: theme.spacing(2),
    fontWeight: 600,
  },
  detailHero: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2.5),
    padding: theme.spacing(2.5),
    borderRadius: 12,
    marginBottom: theme.spacing(2.5),
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
    overflow: 'hidden',
    '@media (max-width: 768px)': {
      flexDirection: 'column !important',
      textAlign: 'center !important',
      padding: '16px 12px !important',
      gap: '12px !important',
    },
  },
  detailHeroCover: {
    width: 140,
    height: 140,
    borderRadius: 10,
    objectFit: 'cover',
    boxShadow: '0 8px 20px rgba(0, 0, 0, 0.3)',
    flexShrink: 0,
    '@media (max-width: 768px)': {
      width: '110px !important',
      height: '110px !important',
    },
  },
  detailHeroInfo: {
    flex: 1,
    minWidth: 0,
    width: '100%',
    boxSizing: 'border-box',
    overflow: 'hidden',
  },
  heroActionsBar: {
    display: 'flex',
    gap: theme.spacing(1.5),
    marginTop: theme.spacing(2),
    '@media (max-width: 768px)': {
      justifyContent: 'center',
    },
  },
  trackItem: {
    display: 'flex',
    alignItems: 'center',
    width: '100%',
    maxWidth: '100%',
    boxSizing: 'border-box',
    padding: '10px 8px',
    borderRadius: 6,
    borderBottom: `1px solid ${theme.palette.divider}`,
    transition: 'background-color 0.15s ease',
    overflow: 'hidden',
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  trackIndex: {
    width: 28,
    minWidth: 28,
    textAlign: 'center',
    color: theme.palette.text.secondary,
    fontSize: '0.85rem',
    marginRight: 6,
    flexShrink: 0,
  },
  trackContent: {
    flex: '1 1 auto',
    minWidth: 0,
    overflow: 'hidden',
    marginRight: 8,
  },
  trackTitle: {
    fontWeight: 600,
    fontSize: '0.9rem',
    color: theme.palette.text.primary,
    lineHeight: 1.4,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  trackMeta: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
    marginTop: 3,
    fontSize: '0.78rem',
    color: theme.palette.text.secondary,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  trackArtist: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    color: '#9CA3AF',
    flex: 1,
    minWidth: 0,
  },
  trackActions: {
    width: 76,
    minWidth: 76,
    flexShrink: 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: 4,
  },
  trackDuration: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    minWidth: 38,
    textAlign: 'right',
  },
}))

const SOURCES = [
  { id: 'wy', label: '☁️ 小芸音乐' },
  { id: 'kw', label: '🐌 小蜗音乐' },
  { id: 'kg', label: '🐕 小构音乐' },
  { id: 'tx', label: '🐧 小秋音乐' },
  { id: 'mg', label: '🍯 小蜜音乐' },
  { id: 'qs', label: '🥤 小气音乐' },
]

export const OnlineMusicPlaylists = () => {
  const classes = useStyles()
  const dispatch = useDispatch()

  const [source, setSource] = useState('wy')
  const [lists, setLists] = useState([])
  const [loading, setLoading] = useState(false)

  // Full-page playlist detail view state
  const [currentPlaylist, setCurrentPlaylist] = useState(null)
  const [detailData, setDetailData] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [actingId, setActingId] = useState(null)

  useEffect(() => {
    let active = true
    setLoading(true)
    getSonglists(source)
      .then((res) => {
        if (active) {
          setLists(res.list || [])
        }
      })
      .catch(() => {})
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [source])

  const handleOpenPlaylist = async (playlist) => {
    setCurrentPlaylist(playlist)
    setDetailLoading(true)
    try {
      const res = await getSonglistDetail(source, playlist.id)
      setDetailData(res)
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to load playlist detail:', err)
      setDetailData(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const handleBackToList = () => {
    setCurrentPlaylist(null)
    setDetailData(null)
  }

  const buildClientMediaFile = (track) => {
    const q = track.quality || 'flac'
    return {
      id: track.id,
      title: track.title,
      artist: track.artist || '未知歌手',
      album: track.album || '',
      duration: track.duration || 240,
      path: `online://${track.source}/${track.songId}/${q}`,
      hasCoverArt: !!track.cover,
    }
  }

  const handlePlayTrack = (track, e) => {
    if (e) e.stopPropagation()
    const mf = buildClientMediaFile(track)
    dispatch(playTracks({ [mf.id]: mf }, [mf.id], mf.id))
    ensureShadowTrack(track).catch(() => {})
  }

  const handlePlayAll = () => {
    if (!detailData || !detailData.list || detailData.list.length === 0) return
    const firstTrack = detailData.list[0]
    const firstMf = buildClientMediaFile(firstTrack)
    dispatch(playTracks({ [firstMf.id]: firstMf }, [firstMf.id], firstMf.id))
    ensureShadowTrack(firstTrack).catch(() => {})

    const remaining = detailData.list.slice(1, 30)
    remaining.forEach((track) => {
      const mf = buildClientMediaFile(track)
      dispatch(addTracks({ [mf.id]: mf }, [mf.id]))
      ensureShadowTrack(track).catch(() => {})
    })
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

  const formatDuration = (sec) => {
    if (!sec || isNaN(sec)) return '--:--'
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m < 10 ? '0' : ''}${m}:${s < 10 ? '0' : ''}${s}`
  }

  // If a playlist is selected, render the Page-Level Detail View (下一级菜单，可返回)
  if (currentPlaylist) {
    return (
      <div className={classes.root}>
        <div className={classes.detailPage}>
          <Button
            startIcon={<ArrowBackIcon />}
            variant="outlined"
            size="small"
            onClick={handleBackToList}
            className={classes.backBtn}
          >
            返回精选歌单列表
          </Button>

          {detailLoading ? (
            <div className={classes.loadingContainer}>
              <CircularProgress size={40} />
            </div>
          ) : detailData ? (
            <div>
              <Paper className={classes.detailHero} elevation={0}>
                <img
                  className={classes.detailHeroCover}
                  src={sanitizeImageUrl(
                    detailData.coverUrl ||
                    currentPlaylist.coverUrl ||
                    'https://p2.music.126.net/6y-UleORITEDbvrOLAL-vQ==/109951167667634547.jpg'
                  )}
                  alt="cover"
                />
                <div className={classes.detailHeroInfo}>
                  <Typography variant="h5" style={{ fontWeight: 700, marginBottom: 8 }}>
                    {detailData.title || currentPlaylist.title}
                  </Typography>
                  <Typography variant="body2" color="textSecondary">
                    创建者: {detailData.author || currentPlaylist.author || '网络音乐推荐'} &nbsp;•&nbsp; 共 {detailData.list ? detailData.list.length : 0} 首曲目
                  </Typography>
                  {detailData.desc && (
                    <Typography
                      variant="body2"
                      color="textSecondary"
                      style={{
                        marginTop: 8,
                        maxHeight: 60,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        '-webkit-line-clamp': 2,
                        '-webkit-box-orient': 'vertical',
                      }}
                    >
                      {detailData.desc}
                    </Typography>
                  )}
                  <div className={classes.heroActionsBar}>
                    <Button
                      variant="contained"
                      color="primary"
                      startIcon={<PlayArrowIcon />}
                      onClick={handlePlayAll}
                      disabled={!detailData.list || detailData.list.length === 0}
                    >
                      播放全部
                    </Button>
                  </div>
                </div>
              </Paper>

              <div style={{ width: '100%' }}>
                {detailData.list && detailData.list.length > 0 ? (
                  detailData.list.map((track, idx) => (
                    <div
                      key={track.id || idx}
                      className={classes.trackItem}
                      onClick={(e) => handlePlayTrack(track, e)}
                    >
                      <div className={classes.trackIndex}>
                        {idx + 1}
                      </div>

                      <div className={classes.trackContent}>
                        <div className={classes.trackTitle} title={track.title}>
                          {track.title}
                        </div>
                        <div className={classes.trackMeta}>
                          <QualityBadge quality={track.quality || 'flac'} />
                          <SourceBadge source={track.source} />
                          <span className={classes.trackArtist}>
                            {track.artist || '未知歌手'} {track.album ? `- ${track.album}` : ''}
                          </span>
                        </div>
                      </div>

                      <div className={classes.trackActions}>
                        <span className={classes.trackDuration}>
                          {formatDuration(track.duration)}
                        </span>
                        <IconButton
                          size="small"
                          onClick={(e) => handleAddToPlaylist(track, e)}
                          title="添加到歌单"
                          style={{ padding: 4 }}
                        >
                          {actingId === track.id ? (
                            <CircularProgress size={16} />
                          ) : (
                            <PlaylistAddIcon fontSize="small" />
                          )}
                        </IconButton>
                      </div>
                    </div>
                  ))
                ) : (
                  <Box p={4} textAlign="center">
                    <Typography color="textSecondary">暂无曲目数据</Typography>
                  </Box>
                )}
              </div>
            </div>
          ) : (
            <Box p={4} textAlign="center">
              <Typography color="textSecondary">获取歌单详情失败</Typography>
            </Box>
          )}
        </div>
      </div>
    )
  }

  // Playlist Grid view
  return (
    <div className={classes.root}>
      <Box display="flex" alignItems="center" mb={2}>
        <QueueMusicIcon style={{ marginRight: 8, color: '#3B82F6' }} />
        <Typography variant="h6">精选网络歌单</Typography>
      </Box>

      {/* Desktop Tabs */}
      <Tabs
        value={source}
        onChange={(e, v) => setSource(v)}
        indicatorColor="primary"
        textColor="primary"
        className={classes.sourceTabs}
      >
        {SOURCES.map((s) => (
          <Tab key={s.id} value={s.id} label={s.label} />
        ))}
      </Tabs>

      {/* Mobile 2-column Grid */}
      <Grid container spacing={1} className={classes.sourceGridMobile}>
        {SOURCES.map((s) => (
          <Grid item xs={6} key={s.id}>
            <Button
              fullWidth
              variant={source === s.id ? 'contained' : 'outlined'}
              color={source === s.id ? 'primary' : 'default'}
              className={classes.sourceCapsule}
              onClick={() => setSource(s.id)}
            >
              {s.label}
            </Button>
          </Grid>
        ))}
      </Grid>

      {loading ? (
        <div className={classes.loadingContainer}>
          <CircularProgress size={40} />
        </div>
      ) : (
        <Grid container spacing={3}>
          {lists.map((item) => (
            <Grid item xs={12} sm={6} md={4} lg={3} xl={2} key={item.id}>
              <Card className={classes.card}>
                <CardActionArea onClick={() => handleOpenPlaylist(item)}>
                  <CardMedia
                    className={classes.media}
                    image={sanitizeImageUrl(
                      item.coverUrl ||
                      'https://p2.music.126.net/6y-UleORITEDbvrOLAL-vQ==/109951167667634547.jpg'
                    )}
                    title={item.title}
                  />
                  <CardContent>
                    <Typography className={classes.title} title={item.title}>
                      {item.title}
                    </Typography>
                    {item.author && (
                      <Typography className={classes.author}>
                        创建者: {item.author}
                      </Typography>
                    )}
                  </CardContent>
                </CardActionArea>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}
    </div>
  )
}

export default OnlineMusicPlaylists

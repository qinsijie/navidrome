import React, { useState, useEffect } from 'react'
import { useDispatch } from 'react-redux'
import {
  Card,
  CardContent,
  Typography,
  Grid,
  CardActionArea,
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
import EmojiEventsIcon from '@material-ui/icons/EmojiEvents'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import { playTracks, addTracks, openAddToPlaylist } from '../actions'
import { getLeaderboards, getLeaderboardDetail, ensureShadowTrack } from './api'
import { QualityBadge, SourceBadge } from './QualityBadge'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(2, 3),
    width: '100%',
    maxWidth: '100%',
    margin: 0,
    boxSizing: 'border-box',
    paddingBottom: 110,
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
    background: 'linear-gradient(145deg, rgba(30, 41, 59, 0.7), rgba(15, 23, 42, 0.8))',
    border: `1px solid ${theme.palette.divider}`,
  },
  boardHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    padding: theme.spacing(3),
  },
  boardTitle: {
    fontWeight: 700,
    fontSize: '1.1rem',
    color: theme.palette.text.primary,
  },
  sourceTabs: {
    marginBottom: theme.spacing(3),
    borderBottom: `1px solid ${theme.palette.divider}`,
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
  },
  backBtn: {
    marginBottom: theme.spacing(2),
    fontWeight: 600,
  },
  detailHero: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(3),
    padding: theme.spacing(3),
    borderRadius: 12,
    marginBottom: theme.spacing(3),
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    [theme.breakpoints.down('xs')]: {
      flexDirection: 'column',
      textAlign: 'center',
    },
  },
  detailHeroInfo: {
    flex: 1,
  },
  heroActionsBar: {
    display: 'flex',
    gap: theme.spacing(1.5),
    marginTop: theme.spacing(2),
    [theme.breakpoints.down('xs')]: {
      justifyContent: 'center',
    },
  },
  trackItem: {
    padding: '12px 16px',
    borderRadius: 6,
    borderBottom: `1px solid ${theme.palette.divider}`,
    transition: 'background-color 0.15s ease',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
}))

const SOURCES = [
  { id: 'wy', label: '网易云音乐' },
]

export const OnlineLeaderboards = () => {
  const classes = useStyles()
  const dispatch = useDispatch()

  const [source, setSource] = useState('wy')
  const [boards, setBoards] = useState([])
  const [loading, setLoading] = useState(false)

  // Full-page detail view state
  const [currentBoard, setCurrentBoard] = useState(null)
  const [detailData, setDetailData] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [actingId, setActingId] = useState(null)

  useEffect(() => {
    let active = true
    setLoading(true)
    getLeaderboards(source)
      .then((res) => {
        if (active) {
          setBoards(res.list || [])
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

  const handleOpenBoard = async (board) => {
    setCurrentBoard(board)
    setDetailLoading(true)
    try {
      const res = await getLeaderboardDetail(source, board.id)
      setDetailData(res)
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to load leaderboard detail:', err)
      setDetailData(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const handleBackToList = () => {
    setCurrentBoard(null)
    setDetailData(null)
  }

  const handlePlayTrack = async (track, e) => {
    if (e) e.stopPropagation()
    setActingId(track.id)
    try {
      const res = await ensureShadowTrack(track)
      if (res && res.mediaFile) {
        const mf = res.mediaFile
        dispatch(playTracks({ [mf.id]: mf }, [mf.id], mf.id))
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to play track:', err)
    } finally {
      setActingId(null)
    }
  }

  const handlePlayAll = async () => {
    if (!detailData || !detailData.list || detailData.list.length === 0) return
    const firstTrack = detailData.list[0]
    await handlePlayTrack(firstTrack)

    for (let i = 1; i < Math.min(detailData.list.length, 30); i++) {
      try {
        const res = await ensureShadowTrack(detailData.list[i])
        if (res && res.mediaFile) {
          dispatch(addTracks({ [res.mediaFile.id]: res.mediaFile }, [res.mediaFile.id]))
        }
      } catch (e) {
        // ignore
      }
    }
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

  // If a leaderboard is selected, render Page-Level Detail View (下一级菜单，可返回)
  if (currentBoard) {
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
            返回热门排行榜列表
          </Button>

          {detailLoading ? (
            <div className={classes.loadingContainer}>
              <CircularProgress size={40} />
            </div>
          ) : detailData ? (
            <div>
              <Paper className={classes.detailHero} elevation={0}>
                <EmojiEventsIcon style={{ color: '#F59E0B', fontSize: '5rem', flexShrink: 0 }} />
                <div className={classes.detailHeroInfo}>
                  <Typography variant="h5" style={{ fontWeight: 700, marginBottom: 8 }}>
                    {detailData.name || currentBoard.name}
                  </Typography>
                  <Typography variant="body2" color="textSecondary">
                    权威热度流行排行榜 &nbsp;•&nbsp; 共 {detailData.list ? detailData.list.length : 0} 首曲目
                  </Typography>
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

              <List disablePadding>
                {detailData.list && detailData.list.length > 0 ? (
                  detailData.list.map((track, idx) => (
                    <ListItem
                      key={track.id || idx}
                      button
                      className={classes.trackItem}
                      onClick={(e) => handlePlayTrack(track, e)}
                    >
                      <Typography
                        variant="body2"
                        color="textSecondary"
                        style={{ width: 40, textAlign: 'center', marginRight: 12 }}
                      >
                        {idx + 1}
                      </Typography>
                      <ListItemText
                        primary={
                          <Typography variant="subtitle2" style={{ fontWeight: 600 }}>
                            {track.title}
                          </Typography>
                        }
                        secondary={
                          <Box display="flex" alignItems="center" mt={0.5}>
                            <QualityBadge quality={track.quality || 'flac'} />
                            <SourceBadge source={track.source} />
                            <span style={{ fontSize: '0.8rem', color: '#9CA3AF' }}>
                              {track.artist} {track.album ? `- ${track.album}` : ''}
                            </span>
                          </Box>
                        }
                      />
                      <ListItemSecondaryAction style={{ display: 'flex', alignItems: 'center' }}>
                        <Typography
                          variant="caption"
                          color="textSecondary"
                          style={{ marginRight: 16 }}
                        >
                          {formatDuration(track.duration)}
                        </Typography>
                        <IconButton
                          size="small"
                          onClick={(e) => handleAddToPlaylist(track, e)}
                          title="添加到歌单"
                        >
                          {actingId === track.id ? (
                            <CircularProgress size={16} />
                          ) : (
                            <PlaylistAddIcon fontSize="small" />
                          )}
                        </IconButton>
                      </ListItemSecondaryAction>
                    </ListItem>
                  ))
                ) : (
                  <Box p={4} textAlign="center">
                    <Typography color="textSecondary">暂无榜单数据</Typography>
                  </Box>
                )}
              </List>
            </div>
          ) : (
            <Box p={4} textAlign="center">
              <Typography color="textSecondary">获取排行榜详情失败</Typography>
            </Box>
          )}
        </div>
      </div>
    )
  }

  // Leaderboard Grid view
  return (
    <div className={classes.root}>
      <Box display="flex" alignItems="center" mb={2}>
        <EmojiEventsIcon style={{ marginRight: 8, color: '#F59E0B' }} />
        <Typography variant="h6">热门音乐排行榜</Typography>
      </Box>

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

      {loading ? (
        <div className={classes.loadingContainer}>
          <CircularProgress size={40} />
        </div>
      ) : (
        <Grid container spacing={3}>
          {boards.map((board) => (
            <Grid item xs={12} sm={6} md={4} lg={3} xl={2} key={board.id}>
              <Card className={classes.card}>
                <CardActionArea onClick={() => handleOpenBoard(board)}>
                  <div className={classes.boardHeader}>
                    <EmojiEventsIcon style={{ color: '#F59E0B', fontSize: '2rem' }} />
                    <Typography className={classes.boardTitle}>
                      {board.name}
                    </Typography>
                  </div>
                  <CardContent style={{ paddingTop: 0 }}>
                    <Typography variant="body2" color="textSecondary">
                      实时热度与权威流行榜单
                    </Typography>
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

export default OnlineLeaderboards

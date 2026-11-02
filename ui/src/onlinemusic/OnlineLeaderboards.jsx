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
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from '@material-ui/core'
import EmojiEventsIcon from '@material-ui/icons/EmojiEvents'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import CloseIcon from '@material-ui/icons/Close'
import { playTracks, addTracks, openAddToPlaylist } from '../actions'
import { getLeaderboards, getLeaderboardDetail, ensureShadowTrack } from './api'
import { QualityBadge, SourceBadge } from './QualityBadge'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    maxWidth: 1400,
    margin: '0 auto',
  },
  card: {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    transition: 'transform 0.2s',
    '&:hover': {
      transform: 'translateY(-4px)',
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
  dialogHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2.5),
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  dialogCover: {
    width: 100,
    height: 100,
    borderRadius: 8,
    objectFit: 'cover',
  },
  dialogInfo: {
    flex: 1,
  },
  dialogActionsBar: {
    display: 'flex',
    gap: theme.spacing(1.5),
    marginTop: theme.spacing(1.5),
  },
  trackItem: {
    borderBottom: `1px solid ${theme.palette.divider}`,
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

  // Detail dialog
  const [selectedBoard, setSelectedBoard] = useState(null)
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

  const handleOpenDetail = async (board) => {
    setSelectedBoard(board)
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

  const handleCloseDetail = () => {
    setSelectedBoard(null)
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
            <Grid item xs={12} sm={6} md={3} key={board.id}>
              <Card className={classes.card}>
                <CardActionArea onClick={() => handleOpenDetail(board)}>
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

      {/* Leaderboard Detail Dialog */}
      <Dialog
        open={Boolean(selectedBoard)}
        onClose={handleCloseDetail}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle disableTypography style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6">排行榜详情</Typography>
          <IconButton size="small" onClick={handleCloseDetail}>
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent dividers style={{ padding: 0 }}>
          {detailLoading ? (
            <div className={classes.loadingContainer}>
              <CircularProgress size={36} />
            </div>
          ) : detailData ? (
            <div>
              <div className={classes.dialogHeader}>
                <EmojiEventsIcon style={{ color: '#F59E0B', fontSize: '3rem' }} />
                <div className={classes.dialogInfo}>
                  <Typography variant="h6" style={{ fontWeight: 700 }}>
                    {detailData.name || selectedBoard?.name}
                  </Typography>
                  <Typography variant="body2" color="textSecondary" style={{ marginTop: 4 }}>
                    共 {detailData.list ? detailData.list.length : 0} 首榜单曲目
                  </Typography>
                  <div className={classes.dialogActionsBar}>
                    <Button
                      variant="contained"
                      color="primary"
                      size="small"
                      startIcon={<PlayArrowIcon />}
                      onClick={handlePlayAll}
                      disabled={!detailData.list || detailData.list.length === 0}
                    >
                      播放全部
                    </Button>
                  </div>
                </div>
              </div>

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
                        style={{ width: 36, textAlign: 'center', marginRight: 12 }}
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
                          style={{ marginRight: 12 }}
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
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDetail} color="primary">
            关闭
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default OnlineLeaderboards

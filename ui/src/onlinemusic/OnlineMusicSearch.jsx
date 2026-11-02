import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch } from 'react-redux'
import {
  Card,
  CardContent,
  Typography,
  TextField,
  Tabs,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Tooltip,
  CircularProgress,
  makeStyles,
  Box,
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import SearchIcon from '@material-ui/icons/Search'
import { playTracks, addTracks, openAddToPlaylist } from '../actions'
import { searchOnline, ensureShadowTrack } from './api'
import QualityBadge from './QualityBadge'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    maxWidth: 1400,
    margin: '0 auto',
  },
  searchBar: {
    marginBottom: theme.spacing(2),
  },
  sourceTabs: {
    marginBottom: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  table: {
    minWidth: 650,
  },
  row: {
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  trackTitle: {
    fontWeight: 600,
    color: theme.palette.text.primary,
  },
  artistName: {
    color: theme.palette.text.secondary,
    fontSize: '0.875rem',
  },
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    padding: theme.spacing(6),
  },
  emptyContainer: {
    textAlign: 'center',
    padding: theme.spacing(6),
    color: theme.palette.text.secondary,
  },
  actionCell: {
    whiteSpace: 'nowrap',
    width: 140,
  },
}))

const SOURCES = [
  { id: 'wy', label: '网易云音乐' },
  { id: 'tx', label: 'QQ 音乐' },
  { id: 'kg', label: '酷狗音乐' },
  { id: 'kw', label: '酷我音乐' },
  { id: 'mg', label: '咪咕音乐' },
]

export const OnlineMusicSearch = () => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const [source, setSource] = useState('wy')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState([])
  const [actingId, setActingId] = useState(null)

  const doSearch = useCallback(
    async (src, query) => {
      if (!query || query.trim() === '') {
        setResults([])
        return
      }
      setLoading(true)
      try {
        const res = await searchOnline(src, query.trim(), 1, 30)
        setResults(res.list || [])
      } catch (err) {
        console.error('Search error:', err)
        setResults([])
      } finally {
        setLoading(false)
      }
    },
    [],
  )

  // 300ms debounce
  useEffect(() => {
    const handler = setTimeout(() => {
      if (keyword.trim()) {
        doSearch(source, keyword)
      }
    }, 300)
    return () => clearTimeout(handler)
  }, [keyword, source, doSearch])

  const handleSourceChange = (event, newSource) => {
    setSource(newSource)
    if (keyword.trim()) {
      doSearch(newSource, keyword)
    }
  }

  const formatDuration = (sec) => {
    if (!sec || isNaN(sec)) return '--:--'
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m}:${s < 10 ? '0' : ''}${s}`
  }

  const handlePlay = async (track) => {
    setActingId(track.id)
    try {
      const res = await ensureShadowTrack(track)
      if (res && res.mediaFile) {
        const mf = res.mediaFile
        dispatch(playTracks({ [mf.id]: mf }, [mf.id], mf.id))
      }
    } catch (err) {
      console.error('Failed to play online track:', err)
    } finally {
      setActingId(null)
    }
  }

  const handleAddToQueue = async (track) => {
    setActingId(track.id)
    try {
      const res = await ensureShadowTrack(track)
      if (res && res.mediaFile) {
        const mf = res.mediaFile
        dispatch(addTracks({ [mf.id]: mf }, [mf.id]))
      }
    } catch (err) {
      console.error('Failed to add to queue:', err)
    } finally {
      setActingId(null)
    }
  }

  const handleAddToPlaylist = async (track) => {
    setActingId(track.id)
    try {
      const res = await ensureShadowTrack(track)
      if (res && res.mediaFile) {
        dispatch(openAddToPlaylist({ selectedIds: [res.mediaFile.id] }))
      }
    } catch (err) {
      console.error('Failed to add to playlist:', err)
    } finally {
      setActingId(null)
    }
  }

  return (
    <div className={classes.root}>
      <Card>
        <CardContent>
          <Box display="flex" alignItems="center" mb={2}>
            <SearchIcon style={{ marginRight: 8, color: '#10B981' }} />
            <Typography variant="h6">网络音乐聚合搜索</Typography>
          </Box>

          <TextField
            className={classes.searchBar}
            fullWidth
            variant="outlined"
            placeholder="搜索歌曲、歌手、专辑..."
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
          />

          <Tabs
            value={source}
            onChange={handleSourceChange}
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
          ) : results.length > 0 ? (
            <TableContainer>
              <Table className={classes.table} size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>歌曲</TableCell>
                    <TableCell>歌手</TableCell>
                    <TableCell>专辑</TableCell>
                    <TableCell align="center">音质</TableCell>
                    <TableCell align="center">时长</TableCell>
                    <TableCell align="right">操作</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {results.map((track) => (
                    <TableRow key={track.id} className={classes.row}>
                      <TableCell>
                        <span className={classes.trackTitle}>{track.title}</span>
                      </TableCell>
                      <TableCell>
                        <span className={classes.artistName}>{track.artist}</span>
                      </TableCell>
                      <TableCell>{track.album || '-'}</TableCell>
                      <TableCell align="center">
                        <QualityBadge quality={track.quality || 'flac'} />
                      </TableCell>
                      <TableCell align="center">
                        {formatDuration(track.duration)}
                      </TableCell>
                      <TableCell align="right" className={classes.actionCell}>
                        <Tooltip title="立即播放">
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handlePlay(track)}
                              disabled={actingId === track.id}
                            >
                              <PlayArrowIcon />
                            </IconButton>
                          </span>
                        </Tooltip>
                        <Tooltip title="添加到播放队列">
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handleAddToQueue(track)}
                              disabled={actingId === track.id}
                            >
                              <QueueMusicIcon />
                            </IconButton>
                          </span>
                        </Tooltip>
                        <Tooltip title="添加到自建歌单">
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handleAddToPlaylist(track)}
                              disabled={actingId === track.id}
                            >
                              <PlaylistAddIcon />
                            </IconButton>
                          </span>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          ) : (
            <div className={classes.emptyContainer}>
              <Typography variant="body1">
                {keyword.trim()
                  ? '未搜索到相关曲目，请尝试切换音源平台或更换关键词'
                  : '请输入关键词开始搜索网络高品质音乐'}
              </Typography>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export default OnlineMusicSearch

import React, { useState, useEffect } from 'react'
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
} from '@material-ui/core'
import EmojiEventsIcon from '@material-ui/icons/EmojiEvents'
import { getLeaderboards } from './api'

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
}))

const SOURCES = [
  { id: 'wy', label: '网易云音乐' },
  { id: 'tx', label: 'QQ 音乐' },
  { id: 'kg', label: '酷狗音乐' },
  { id: 'kw', label: '酷我音乐' },
]

export const OnlineLeaderboards = () => {
  const classes = useStyles()
  const [source, setSource] = useState('wy')
  const [boards, setBoards] = useState([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let active = true
    setLoading(true)
    getLeaderboards(source)
      .then((res) => {
        if (active) {
          setBoards(res.list || [])
        }
      })
      .catch((err) => console.error(err))
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [source])

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
          {boards.map((b) => (
            <Grid item xs={12} sm={6} md={3} key={b.id}>
              <Card className={classes.card}>
                <CardActionArea className={classes.boardHeader}>
                  <EmojiEventsIcon style={{ fontSize: 36, color: '#F59E0B' }} />
                  <div>
                    <Typography className={classes.boardTitle}>
                      {b.name}
                    </Typography>
                    <Typography variant="caption" color="textSecondary">
                      每日实时更新
                    </Typography>
                  </div>
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

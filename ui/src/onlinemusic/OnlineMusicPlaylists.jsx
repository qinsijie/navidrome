import React, { useState, useEffect } from 'react'
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
} from '@material-ui/core'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import { getSonglists } from './api'

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

export const OnlineMusicPlaylists = () => {
  const classes = useStyles()
  const [source, setSource] = useState('wy')
  const [lists, setLists] = useState([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let active = true
    setLoading(true)
    getSonglists(source)
      .then((res) => {
        if (active) {
          setLists(res.list || [])
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
        <QueueMusicIcon style={{ marginRight: 8, color: '#3B82F6' }} />
        <Typography variant="h6">精选网络歌单</Typography>
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
          {lists.map((item) => (
            <Grid item xs={12} sm={6} md={4} lg={3} key={item.id}>
              <Card className={classes.card}>
                <CardActionArea>
                  <CardMedia
                    className={classes.media}
                    image={
                      item.coverUrl ||
                      'https://p2.music.126.net/6y-UleORITEDbvrOLAL-vQ==/109951167667634547.jpg'
                    }
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

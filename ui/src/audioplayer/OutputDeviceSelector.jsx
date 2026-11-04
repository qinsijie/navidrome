import React, { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Typography,
  Tooltip,
  Divider,
  ButtonBase,
  useTheme,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdSpeaker,
  MdComputer,
  MdCheck,
  MdTune,
  MdSettings,
  MdArrowForward,
} from 'react-icons/md'
import { useNotify } from 'react-admin'
import { httpClient } from '../dataProvider'
import HQPlayerTakeoverDialog from './HQPlayerTakeoverDialog'

const useStyles = makeStyles((theme) => ({
  zoneBadge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '3px 10px',
    borderRadius: 14,
    border: (props) =>
      props.isDark
        ? '1px solid rgba(255, 255, 255, 0.28) !important'
        : '1px solid rgba(0, 0, 0, 0.25) !important',
    backgroundColor: (props) =>
      props.isDark
        ? 'rgba(255, 255, 255, 0.12) !important'
        : 'rgba(0, 0, 0, 0.08) !important',
    color: (props) => (props.isDark ? '#ffffff !important' : '#1e293b !important'),
    fontSize: '0.8rem',
    fontWeight: 600,
    cursor: 'pointer',
    maxWidth: 170,
    whiteSpace: 'nowrap',
    transition: 'all 0.2s ease',
    '@media (max-width: 768px)': {
      padding: '4px 6px !important',
      gap: '0 !important',
      borderRadius: '8px !important',
    },
    '&:hover': {
      backgroundColor: (props) =>
        props.isDark
          ? 'rgba(255, 255, 255, 0.22) !important'
          : 'rgba(0, 0, 0, 0.15) !important',
      borderColor: `${theme.palette.primary.main} !important`,
    },
    '& svg': {
      color: (props) =>
        props.isDark ? '#90caf9 !important' : '#1976d2 !important',
      fill: (props) =>
        props.isDark ? '#90caf9 !important' : '#1976d2 !important',
      flexShrink: 0,
    },
  },
  badgeText: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: 110,
    fontSize: '0.8rem',
    fontWeight: 600,
    color: (props) => (props.isDark ? '#ffffff !important' : '#1e293b !important'),
    '@media (max-width: 768px)': {
      display: 'none !important',
    },
  },
  menuHeader: {
    padding: '8px 16px',
    fontWeight: 'bold',
    fontSize: '0.85rem',
    color: theme.palette.text.secondary,
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  deviceItem: {
    minWidth: 280,
    padding: '8px 16px',
  },
  activeText: {
    color: theme.palette.primary.main,
    fontWeight: 'bold',
  },
  descText: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    display: 'block',
  },
  nowPlayingText: {
    fontSize: '0.75rem',
    color: '#31c27c',
    fontWeight: 500,
    display: 'block',
  },
  footerItem: {
    padding: '10px 16px',
    color: theme.palette.primary.main,
    fontSize: '0.85rem',
    fontWeight: 500,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    textDecoration: 'none',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
}))

const OutputDeviceSelector = ({ isDesktop, className }) => {
  const theme = useTheme()
  const isDark = theme.palette.type === 'dark'
  const classes = useStyles({ isDark })
  const notify = useNotify()
  const [anchorEl, setAnchorEl] = useState(null)
  const [devices, setDevices] = useState([])
  const [activeId, setActiveId] = useState('local')
  const [nowPlaying, setNowPlaying] = useState(null)
  const [takeoverDialogOpen, setTakeoverDialogOpen] = useState(false)
  const [occupiedBy, setOccupiedBy] = useState(null)
  const [pendingDev, setPendingDev] = useState(null)

  const loadDevices = useCallback(() => {
    httpClient('/api/output/devices?enabled_only=true')
      .then(({ json }) => {
        if (json) {
          if (json.devices) setDevices(json.devices)
          if (json.nowPlaying) {
            setNowPlaying(json.nowPlaying)
          }
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    loadDevices()
  }, [loadDevices])

  const handleClick = (event) => {
    setAnchorEl(event.currentTarget)
    loadDevices()
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const doSelectDevice = (dev, forceTakeover = false) => {
    httpClient('/api/output/device', {
      method: 'POST',
      body: JSON.stringify({ deviceId: dev.id, forceTakeover }),
    })
      .then(() => {
        setActiveId(dev.id)
        window.dispatchEvent(
          new CustomEvent('nd:output-device-changed', { detail: { deviceId: dev.id, device: dev } }),
        )
        notify(`已选择输出设备: ${dev.name} (点击播放按钮时推流)`, 'info')
        setTakeoverDialogOpen(false)
        handleClose()
      })
      .catch((err) => {
        if (err.status === 409 || err.body?.code === 'HQPLAYER_OCCUPIED') {
          const occ = err.body?.occupiedBy || { userName: '其他用户', trackTitle: '外部音频流' }
          setOccupiedBy(occ)
          setPendingDev(dev)
          setTakeoverDialogOpen(true)
          handleClose()
          return
        }
        notify(`切换输出设备失败: ${err.message}`, 'error')
      })
  }

  const handleSelectDevice = (dev) => {
    if (dev.id === 'local') {
      // Local browser / device playback is private to current device
      setActiveId('local')
      window.dispatchEvent(
        new CustomEvent('nd:output-device-changed', { detail: { deviceId: 'local', device: dev } }),
      )
      // Best-effort notify server and immediately release remote audio hardware
      httpClient('/api/output/device', {
        method: 'POST',
        body: JSON.stringify({ deviceId: 'local' }),
      }).catch(() => {})
      httpClient('/api/output/control', {
        method: 'POST',
        body: JSON.stringify({ action: 'stop' }),
      }).catch(() => {})
      notify('音频输出已切换至: 本机播放 (已释放远端设备)', 'info')
      handleClose()
      return
    }

    doSelectDevice(dev, false)
  }

  const activeDevice = devices.find((d) => d.id === activeId) || {
    id: 'local',
    name: '本机播放',
    type: 'local',
  }

  const getDeviceIcon = (dev) => {
    if (dev.type === 'hqplayer') return <MdTune size={18} />
    if (dev.type === 'dlna') return <MdSpeaker size={18} />
    return <MdComputer size={18} />
  }

  return (
    <>
      <Tooltip title="音频输出设备切换 (Zone Picker)">
        <ButtonBase
          onClick={handleClick}
          className={`${classes.zoneBadge} ${className || ''}`}
          data-testid="output-device-selector"
        >
          {getDeviceIcon(activeDevice)}
          <span className={classes.badgeText}>{activeDevice.name || '本机播放'}</span>
        </ButtonBase>
      </Tooltip>

      <Menu
        anchorEl={anchorEl}
        keepMounted
        open={Boolean(anchorEl)}
        onClose={handleClose}
        getContentAnchorEl={null}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <div className={classes.menuHeader}>音频输出分区 (Audio Zones)</div>
        {devices.map((dev) => {
          const isActive = dev.id === activeId
          const isDevPlaying =
            isActive &&
            nowPlaying &&
            nowPlaying.deviceId === dev.id &&
            nowPlaying.track &&
            nowPlaying.track.title
          return (
            <MenuItem
              key={dev.id}
              onClick={() => handleSelectDevice(dev)}
              className={classes.deviceItem}
            >
              <ListItemIcon>{getDeviceIcon(dev)}</ListItemIcon>
              <ListItemText
                primary={
                  <Typography
                    variant="body2"
                    className={isActive ? classes.activeText : undefined}
                  >
                    {dev.name}
                  </Typography>
                }
                secondary={
                  isDevPlaying ? (
                    <span className={classes.nowPlayingText}>
                      ▶ 正在播放: {nowPlaying.track.title} - {nowPlaying.track.artist}
                    </span>
                  ) : (
                    <span className={classes.descText}>{dev.description}</span>
                  )
                }
              />
              {isActive && <MdCheck color="#31c27c" size={20} />}
            </MenuItem>
          )
        })}

        <Divider />
        <MenuItem
          component={Link}
          to="/settings/audio"
          onClick={handleClose}
          className={classes.footerItem}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <MdSettings size={18} />
            <span>前往音频设置配置新设备</span>
          </div>
          <MdArrowForward size={16} />
        </MenuItem>
      </Menu>

      <HQPlayerTakeoverDialog
        open={takeoverDialogOpen}
        onClose={() => setTakeoverDialogOpen(false)}
        onConfirm={() => {
          if (pendingDev) {
            doSelectDevice(pendingDev, true)
          }
        }}
        occupiedBy={occupiedBy}
      />
    </>
  )
}

export default OutputDeviceSelector

import React, { useCallback, useEffect, useState } from 'react'
import {
  IconButton,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Typography,
  Tooltip,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdDevices,
  MdSpeaker,
  MdComputer,
  MdCheck,
  MdTune,
  MdSettings,
} from 'react-icons/md'
import { useNotify } from 'react-admin'
import { httpClient } from '../dataProvider'

const useStyles = makeStyles((theme) => ({
  menuHeader: {
    padding: '8px 16px',
    fontWeight: 'bold',
    fontSize: '0.875rem',
    color: theme.palette.text.secondary,
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  deviceItem: {
    minWidth: 260,
    padding: '8px 16px',
  },
  activeText: {
    color: theme.palette.primary.main,
    fontWeight: 'bold',
  },
  descText: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
  },
  configRow: {
    borderTop: `1px solid ${theme.palette.divider}`,
    marginTop: 4,
  },
}))

const OutputDeviceSelector = ({ isDesktop, className }) => {
  const classes = useStyles()
  const notify = useNotify()
  const [anchorEl, setAnchorEl] = useState(null)
  const [devices, setDevices] = useState([])
  const [activeId, setActiveId] = useState('local')
  const [hqConfig, setHqConfig] = useState({ host: '127.0.0.1', port: 32400, enabled: false })
  const [dialogOpen, setDialogOpen] = useState(false)
  const [hqForm, setHqForm] = useState({ host: '', port: 32400, enabled: true })

  const loadDevices = useCallback(() => {
    httpClient('/api/output/devices')
      .then(({ json }) => {
        if (json) {
          if (json.devices) setDevices(json.devices)
          if (json.activeId) setActiveId(json.activeId)
          if (json.hqConfig) {
            setHqConfig(json.hqConfig)
            setHqForm({
              host: json.hqConfig.host || '',
              port: json.hqConfig.port || 32400,
              enabled: json.hqConfig.enabled || false,
            })
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

  const handleSelectDevice = (dev) => {
    httpClient('/api/output/device', {
      method: 'POST',
      body: JSON.stringify({ deviceId: dev.id }),
    })
      .then(() => {
        setActiveId(dev.id)
        notify(`已切换音频输出至: ${dev.name}`, 'info')
        handleClose()
      })
      .catch((err) => {
        notify(`切换输出设备失败: ${err.message}`, 'error')
      })
  }

  const handleSaveHqConfig = () => {
    httpClient('/api/output/hqplayer/config', {
      method: 'POST',
      body: JSON.stringify({
        host: hqForm.host,
        port: parseInt(hqForm.port, 10) || 32400,
        enabled: hqForm.enabled,
      }),
    })
      .then(() => {
        notify('HQPlayer 配置已更新', 'info')
        setDialogOpen(false)
        loadDevices()
      })
      .catch((err) => {
        notify(`保存失败: ${err.message}`, 'error')
      })
  }

  const getDeviceIcon = (dev) => {
    if (dev.type === 'hqplayer') return <MdTune size={20} />
    if (dev.type === 'dlna') return <MdSpeaker size={20} />
    return <MdComputer size={20} />
  }

  return (
    <>
      <Tooltip title="音频输出设备 (DLNA / HQPlayer)">
        <IconButton
          size={isDesktop ? 'small' : undefined}
          onClick={handleClick}
          className={className}
          data-testid="output-device-selector"
        >
          <MdDevices size={isDesktop ? 20 : 18} />
        </IconButton>
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
        <div className={classes.menuHeader}>Hi-Fi 音频输出设备</div>
        {devices.map((dev) => {
          const isActive = dev.id === activeId
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
                  <span className={classes.descText}>{dev.description}</span>
                }
              />
              {isActive && <MdCheck color="#31c27c" size={18} />}
            </MenuItem>
          )
        })}

        <MenuItem
          onClick={() => {
            handleClose()
            setDialogOpen(true)
          }}
          className={`${classes.deviceItem} ${classes.configRow}`}
        >
          <ListItemIcon>
            <MdSettings size={20} />
          </ListItemIcon>
          <ListItemText
            primary={<Typography variant="body2">配置 HQPlayer 节点</Typography>}
            secondary={<span className={classes.descText}>设置 IP 与控制端口直推</span>}
          />
        </MenuItem>
      </Menu>

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>配置 HQPlayer 源码直推节点</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="textSecondary" style={{ marginBottom: 16 }}>
            启用后 Navidrome 将旁路重采样与转码流程，将高解析音频流 (PCM / DSD) 直推至 HQPlayer 引擎。
          </Typography>
          <TextField
            label="HQPlayer IP / 主机名"
            fullWidth
            margin="dense"
            variant="outlined"
            value={hqForm.host}
            onChange={(e) => setHqForm({ ...hqForm, host: e.target.value })}
            placeholder="192.168.1.100"
          />
          <TextField
            label="控制端口"
            fullWidth
            margin="dense"
            variant="outlined"
            type="number"
            value={hqForm.port}
            onChange={(e) => setHqForm({ ...hqForm, port: e.target.value })}
            placeholder="32400"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>取消</Button>
          <Button
            onClick={handleSaveHqConfig}
            color="primary"
            variant="contained"
          >
            保存并启用
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default OutputDeviceSelector

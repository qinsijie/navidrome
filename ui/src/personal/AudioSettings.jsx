import React, { useState, useEffect, useCallback } from 'react'
import { Title, useNotify } from 'react-admin'
import {
  Card,
  CardContent,
  CardHeader,
  Typography,
  TextField,
  Switch,
  FormControlLabel,
  Button,
  IconButton,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  ListItemSecondaryAction,
  Chip,
  CircularProgress,
  Grid,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdSpeaker,
  MdComputer,
  MdTune,
  MdRefresh,
  MdAdd,
  MdDelete,
  MdCheckCircle,
  MdError,
  MdSettings,
} from 'react-icons/md'
import { httpClient } from '../dataProvider'

const useStyles = makeStyles((theme) => ({
  root: {
    maxWidth: 960,
    margin: '24px auto',
    padding: '0 16px',
    paddingBottom: 96,
  },
  card: {
    marginBottom: 24,
    borderRadius: 8,
    boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
  },
  headerIcon: {
    verticalAlign: 'middle',
    marginRight: 8,
    color: theme.palette.primary.main,
  },
  actionRow: {
    marginTop: 16,
    display: 'flex',
    alignItems: 'center',
    gap: 12,
  },
  statusText: {
    marginLeft: 12,
    fontSize: '0.875rem',
    display: 'inline-flex',
    alignItems: 'center',
    gap: 4,
  },
  successText: {
    color: '#31c27c',
    fontWeight: 500,
  },
  errorText: {
    color: theme.palette.error.main,
    fontWeight: 500,
  },
  dlnaActions: {
    display: 'flex',
    gap: 12,
    marginBottom: 16,
  },
  listItem: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    '&:last-child': {
      borderBottom: 'none',
    },
  },
  deviceChip: {
    marginLeft: 8,
    height: 20,
    fontSize: '0.75rem',
  },
}))

const AudioSettings = () => {
  const classes = useStyles()
  const notify = useNotify()

  const [loading, setLoading] = useState(false)
  const [localName, setLocalName] = useState('本机')
  const [localEnabled, setLocalEnabled] = useState(true)

  const [hqConfig, setHqConfig] = useState({
    name: 'HQPlayer - 客厅',
    host: '',
    port: 32400,
    enabled: false,
  })
  const [hqTesting, setHqTesting] = useState(false)
  const [hqTestResult, setHqTestResult] = useState(null)

  const [dlnaDevices, setDlnaDevices] = useState([])
  const [scanning, setScanning] = useState(false)

  // Manual DLNA dialog
  const [manualDialogOpen, setManualDialogOpen] = useState(false)
  const [manualName, setManualName] = useState('')
  const [manualUrl, setManualUrl] = useState('')
  const [addingManual, setAddingManual] = useState(false)

  const loadData = useCallback(() => {
    setLoading(true)
    httpClient('/api/output/devices?enabled_only=false')
      .then(({ json }) => {
        if (json) {
          const devList = json.devices || []
          const localDev = devList.find((d) => d.type === 'local')
          if (localDev) {
            setLocalName(localDev.name || '本机')
            setLocalEnabled(localDev.enabled !== false)
          }

          if (json.hqConfig) {
            setHqConfig({
              name: json.hqConfig.name || 'HQPlayer - 客厅',
              host: json.hqConfig.host || '',
              port: json.hqConfig.port || 32400,
              enabled: json.hqConfig.enabled || false,
            })
          }

          const dlnaList = devList.filter((d) => d.type === 'dlna')
          setDlnaDevices(dlnaList)
        }
      })
      .catch((err) => {
        notify(`获取音频设备失败: ${err.message}`, 'error')
      })
      .finally(() => {
        setLoading(false)
      })
  }, [notify])

  useEffect(() => {
    loadData()
  }, [loadData])

  // Save Local config
  const handleSaveLocal = () => {
    httpClient('/api/output/device/update', {
      method: 'POST',
      body: JSON.stringify({
        deviceId: 'local',
        name: localName,
        enabled: localEnabled,
      }),
    })
      .then(() => {
        notify('本机播放配置已保存', 'info')
        loadData()
      })
      .catch((err) => {
        notify(`保存失败: ${err.message}`, 'error')
      })
  }

  // Save HQPlayer config
  const handleSaveHQ = () => {
    httpClient('/api/output/hqplayer/config', {
      method: 'POST',
      body: JSON.stringify({
        name: hqConfig.name,
        host: hqConfig.host.trim(),
        port: parseInt(hqConfig.port, 10) || 32400,
        enabled: hqConfig.enabled,
      }),
    })
      .then(() => {
        notify('HQPlayer 配置已保存', 'info')
        loadData()
      })
      .catch((err) => {
        notify(`保存失败: ${err.message}`, 'error')
      })
  }

  // Test HQPlayer connection
  const handleTestHQ = () => {
    if (!hqConfig.host.trim()) {
      notify('请输入 HQPlayer IP 地址', 'warning')
      return
    }
    setHqTesting(true)
    setHqTestResult(null)
    httpClient('/api/output/hqplayer/test', {
      method: 'POST',
      body: JSON.stringify({
        host: hqConfig.host.trim(),
        port: parseInt(hqConfig.port, 10) || 32400,
      }),
    })
      .then(({ json }) => {
        if (json && json.ok) {
          setHqTestResult({ ok: true, msg: json.message || '握手成功: HQPlayer 运行中' })
        } else {
          setHqTestResult({ ok: false, msg: (json && json.message) || '连接超时或未响应' })
        }
      })
      .catch((err) => {
        setHqTestResult({ ok: false, msg: err.message || '网络连接错误' })
      })
      .finally(() => {
        setHqTesting(false)
      })
  }

  // Scan DLNA devices
  const handleScanDLNA = () => {
    setScanning(true)
    httpClient('/api/output/dlna/scan', { method: 'POST' })
      .then(({ json }) => {
        if (json && json.devices) {
          const list = json.devices.filter((d) => d.type === 'dlna')
          setDlnaDevices(list)
          notify(`扫描完成，发现 ${list.length} 个 DLNA 播放器`, 'info')
        }
      })
      .catch((err) => {
        notify(`扫描失败: ${err.message}`, 'error')
      })
      .finally(() => {
        setScanning(false)
      })
  }

  // Toggle DLNA device enable
  const handleToggleDLNA = (dev) => {
    const nextEnabled = !dev.enabled
    httpClient('/api/output/device/update', {
      method: 'POST',
      body: JSON.stringify({
        deviceId: dev.id,
        name: dev.name,
        enabled: nextEnabled,
      }),
    })
      .then(() => {
        setDlnaDevices((prev) =>
          prev.map((d) => (d.id === dev.id ? { ...d, enabled: nextEnabled } : d)),
        )
        notify(`已${nextEnabled ? '启用' : '停用'}设备: ${dev.name}`, 'info')
      })
      .catch((err) => {
        notify(`更新失败: ${err.message}`, 'error')
      })
  }

  // Delete DLNA device
  const handleDeleteDLNA = (devId) => {
    httpClient(`/api/output/dlna/${encodeURIComponent(devId)}`, {
      method: 'DELETE',
    })
      .then(() => {
        setDlnaDevices((prev) => prev.filter((d) => d.id !== devId))
        notify('设备已从列表中移除', 'info')
      })
      .catch((err) => {
        notify(`移除失败: ${err.message}`, 'error')
      })
  }

  // Add Manual DLNA
  const handleAddManualDLNA = () => {
    if (!manualUrl.trim()) {
      notify('请输入设备描述文件 XML URL 地址', 'warning')
      return
    }
    setAddingManual(true)
    httpClient('/api/output/dlna/add', {
      method: 'POST',
      body: JSON.stringify({
        name: manualName.trim(),
        url: manualUrl.trim(),
      }),
    })
      .then(() => {
        notify('网络设备添加成功', 'info')
        setManualDialogOpen(false)
        setManualName('')
        setManualUrl('')
        loadData()
      })
      .catch((err) => {
        notify(`添加失败: ${err.message}`, 'error')
      })
      .finally(() => {
        setAddingManual(false)
      })
  }

  return (
    <div className={classes.root}>
      <Title title="Navidrome - 音频输出与串流管理 (Zone Management)" />
      <div style={{ marginBottom: 24 }}>
        <Typography variant="h5" style={{ fontWeight: 600 }}>
          <MdSettings className={classes.headerIcon} />
          音频输出分区管理 (Audio Zones)
        </Typography>
        <Typography variant="body2" color="textSecondary" style={{ marginTop: 4 }}>
          参考 Roon 架构设计，集中管理所有本地声卡、HQPlayer 串流直推节点以及 DLNA / UPnP 局域网音频设备。启用的设备将在底部播放条的 Zone Picker 中快速切换。
        </Typography>
      </div>

      {/* 1. 本机播放 */}
      <Card className={classes.card}>
        <CardHeader
          title={
            <Typography variant="h6" style={{ fontWeight: 600 }}>
              <MdComputer className={classes.headerIcon} />
              本机播放 (Web Audio / App 内置引擎)
            </Typography>
          }
          subheader="控制当前浏览器端或客户端是否直接输出声音"
        />
        <Divider />
        <CardContent>
          <Grid container spacing={3} alignItems="center">
            <Grid item xs={12} sm={6}>
              <TextField
                label="显示别名"
                fullWidth
                variant="outlined"
                size="small"
                value={localName}
                onChange={(e) => setLocalName(e.target.value)}
                placeholder="本机"
              />
            </Grid>
            <Grid item xs={12} sm={3}>
              <FormControlLabel
                control={
                  <Switch
                    checked={localEnabled}
                    onChange={(e) => setLocalEnabled(e.target.checked)}
                    color="primary"
                  />
                }
                label={localEnabled ? '已启用' : '已禁用'}
              />
            </Grid>
            <Grid item xs={12} sm={3}>
              <Button
                variant="contained"
                color="primary"
                onClick={handleSaveLocal}
                disabled={loading}
              >
                保存别名
              </Button>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      {/* 2. HQPlayer 节点 */}
      <Card className={classes.card}>
        <CardHeader
          title={
            <Typography variant="h6" style={{ fontWeight: 600 }}>
              <MdTune className={classes.headerIcon} />
              HQPlayer Hi-Fi 直推节点 (Raw Stream Pass-through)
            </Typography>
          }
          subheader="旁路服务端转码与重采样，直推高解析 PCM / DSD 源码至 HQPlayer Embedded 或 Desktop 引擎"
        />
        <Divider />
        <CardContent>
          <Grid container spacing={3}>
            <Grid item xs={12} sm={4}>
              <TextField
                label="节点名称"
                fullWidth
                variant="outlined"
                size="small"
                value={hqConfig.name}
                onChange={(e) => setHqConfig({ ...hqConfig, name: e.target.value })}
                placeholder="HQPlayer - 客厅"
              />
            </Grid>
            <Grid item xs={12} sm={5}>
              <TextField
                label="HQPlayer IP / 主机名"
                fullWidth
                variant="outlined"
                size="small"
                value={hqConfig.host}
                onChange={(e) => setHqConfig({ ...hqConfig, host: e.target.value })}
                placeholder="例如 192.168.1.100"
              />
            </Grid>
            <Grid item xs={12} sm={3}>
              <TextField
                label="控制端口"
                fullWidth
                variant="outlined"
                size="small"
                type="number"
                value={hqConfig.port}
                onChange={(e) => setHqConfig({ ...hqConfig, port: e.target.value })}
                placeholder="32400"
              />
            </Grid>
          </Grid>

          <div className={classes.actionRow}>
            <FormControlLabel
              control={
                <Switch
                  checked={hqConfig.enabled}
                  onChange={(e) => setHqConfig({ ...hqConfig, enabled: e.target.checked })}
                  color="primary"
                />
              }
              label={hqConfig.enabled ? '已启用该节点' : '未启用'}
            />
            <Button
              variant="outlined"
              color="primary"
              onClick={handleTestHQ}
              disabled={hqTesting}
              startIcon={hqTesting ? <CircularProgress size={16} /> : undefined}
            >
              {hqTesting ? '测试中...' : '连接测试'}
            </Button>
            <Button
              variant="contained"
              color="primary"
              onClick={handleSaveHQ}
              disabled={loading}
            >
              保存配置
            </Button>

            {hqTestResult && (
              <span
                className={`${classes.statusText} ${
                  hqTestResult.ok ? classes.successText : classes.errorText
                }`}
              >
                {hqTestResult.ok ? <MdCheckCircle size={18} /> : <MdError size={18} />}
                {hqTestResult.msg}
              </span>
            )}
          </div>
        </CardContent>
      </Card>

      {/* 3. DLNA / UPnP 渲染器 */}
      <Card className={classes.card}>
        <CardHeader
          title={
            <Typography variant="h6" style={{ fontWeight: 600 }}>
              <MdSpeaker className={classes.headerIcon} />
              DLNA / UPnP 局域网串流音箱 (AVTransport)
            </Typography>
          }
          subheader="发现并推送无损音频至局域网内的网络音箱、功放、AV 接收机等设备"
        />
        <Divider />
        <CardContent>
          <div className={classes.dlnaActions}>
            <Button
              variant="outlined"
              color="primary"
              startIcon={scanning ? <CircularProgress size={16} /> : <MdRefresh size={18} />}
              onClick={handleScanDLNA}
              disabled={scanning}
            >
              {scanning ? '正在探测局域网...' : '重新扫描局域网设备'}
            </Button>
            <Button
              variant="outlined"
              startIcon={<MdAdd size={18} />}
              onClick={() => setManualDialogOpen(true)}
            >
              手动添加网络设备
            </Button>
          </div>

          <List disablePadding>
            {dlnaDevices.length === 0 ? (
              <ListItem>
                <ListItemText
                  secondary="暂未发现 DLNA / UPnP 渲染设备。请确保网络音箱与服务器位于同一子网并开启 DLNA，或点击“重新扫描”/“手动添加”。"
                />
              </ListItem>
            ) : (
              dlnaDevices.map((dev) => (
                <ListItem key={dev.id} className={classes.listItem}>
                  <ListItemIcon>
                    <MdSpeaker size={28} color="#2196f3" />
                  </ListItemIcon>
                  <ListItemText
                    primary={
                      <div style={{ display: 'flex', alignItems: 'center' }}>
                        <Typography variant="subtitle1" style={{ fontWeight: 600 }}>
                          {dev.name}
                        </Typography>
                        <Chip
                          label={dev.status === 'online' ? '在线' : '离线'}
                          size="small"
                          color={dev.status === 'online' ? 'primary' : 'default'}
                          className={classes.deviceChip}
                        />
                      </div>
                    }
                    secondary={
                      <Typography variant="body2" color="textSecondary" style={{ fontSize: '0.8rem', wordBreak: 'break-all' }}>
                        地址: {dev.url || dev.controlUrl || '未知'}
                      </Typography>
                    }
                  />
                  <ListItemSecondaryAction>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={dev.enabled}
                          onChange={() => handleToggleDLNA(dev)}
                          color="primary"
                        />
                      }
                      label={dev.enabled ? '启用' : '禁用'}
                    />
                    <IconButton
                      edge="end"
                      aria-label="delete"
                      onClick={() => handleDeleteDLNA(dev.id)}
                      size="small"
                    >
                      <MdDelete size={20} />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              ))
            )}
          </List>
        </CardContent>
      </Card>

      {/* Manual DLNA Dialog */}
      <Dialog
        open={manualDialogOpen}
        onClose={() => setManualDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>手动添加 DLNA / UPnP 设备</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="textSecondary" style={{ marginBottom: 16 }}>
            对于跨 VLAN、Docker bridge 隔离环境或禁用了 SSDP 广播的音响，可直接输入其 UPnP 描述文件 URL（例如 <code>http://192.168.1.50:1400/xml/device_description.xml</code>）。
          </Typography>
          <TextField
            label="设备名称 (可选)"
            fullWidth
            variant="outlined"
            margin="dense"
            value={manualName}
            onChange={(e) => setManualName(e.target.value)}
            placeholder="如：客厅 Sonos / 天龙功放"
          />
          <TextField
            label="设备 XML 描述地址 (必填)"
            fullWidth
            variant="outlined"
            margin="dense"
            value={manualUrl}
            onChange={(e) => setManualUrl(e.target.value)}
            placeholder="http://192.168.1.50:8080/description.xml"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setManualDialogOpen(false)}>取消</Button>
          <Button
            onClick={handleAddManualDLNA}
            color="primary"
            variant="contained"
            disabled={addingManual}
            startIcon={addingManual ? <CircularProgress size={16} /> : undefined}
          >
            {addingManual ? '添加中...' : '确认添加'}
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default AudioSettings

import React, { useState, useEffect, useRef } from 'react'
import {
  Card,
  CardContent,
  Typography,
  Button,
  TextField,
  Chip,
  Box,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Radio,
  Tooltip,
  IconButton,
  makeStyles,
} from '@material-ui/core'
import SettingsEthernetIcon from '@material-ui/icons/SettingsEthernet'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import CloudDownloadIcon from '@material-ui/icons/CloudDownload'
import PublishIcon from '@material-ui/icons/Publish'
import DeleteOutlineIcon from '@material-ui/icons/DeleteOutline'
import RefreshIcon from '@material-ui/icons/Refresh'
import SpeedIcon from '@material-ui/icons/Speed'
import LinkIcon from '@material-ui/icons/Link'
import {
  getSources,
  uploadSource,
  importSourceFromUrl,
  activateSource,
  deleteSource,
  checkSourceUpdate,
  pingSources,
} from './api'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    maxWidth: 1000,
    margin: '0 auto',
  },
  headerBox: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(3),
    flexWrap: 'wrap',
    gap: theme.spacing(2),
  },
  actionButtons: {
    display: 'flex',
    gap: theme.spacing(1.5),
    flexWrap: 'wrap',
  },
  card: {
    marginBottom: theme.spacing(2.5),
    borderRadius: 12,
    border: '1px solid',
    borderColor: theme.palette.type === 'dark' ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
    boxShadow: theme.palette.type === 'dark' ? '0 4px 20px rgba(0, 0, 0, 0.3)' : '0 2px 10px rgba(0, 0, 0, 0.05)',
    transition: 'all 0.2s ease',
  },
  activeCard: {
    borderColor: '#10B981',
    boxShadow: theme.palette.type === 'dark' ? '0 4px 24px rgba(16, 185, 129, 0.2)' : '0 4px 16px rgba(16, 185, 129, 0.15)',
  },
  cardHeader: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(1),
  },
  titleGroup: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    flexWrap: 'wrap',
  },
  platformChips: {
    display: 'flex',
    gap: theme.spacing(1),
    marginTop: theme.spacing(1.5),
    flexWrap: 'wrap',
  },
  footerRow: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: theme.spacing(2),
    paddingTop: theme.spacing(1.5),
    borderTop: `1px solid ${theme.palette.divider}`,
    flexWrap: 'wrap',
    gap: theme.spacing(1),
  },
  pingCard: {
    marginBottom: theme.spacing(3),
    padding: theme.spacing(2),
    borderRadius: 12,
    background: theme.palette.type === 'dark' ? 'rgba(255, 255, 255, 0.03)' : 'rgba(0, 0, 0, 0.02)',
    border: `1px solid ${theme.palette.divider}`,
  },
}))

const PLATFORM_MAP = {
  wy: { name: '网易云', color: '#e60026' },
  tx: { name: 'QQ音乐', color: '#10b981' },
  kg: { name: '酷狗音乐', color: '#2563eb' },
  kw: { name: '酷我音乐', color: '#f59e0b' },
  mg: { name: '咪咕音乐', color: '#ec4899' },
}

export const SourceSettings = () => {
  const classes = useStyles()
  const [sources, setSources] = useState([])
  const [activeMeta, setActiveMeta] = useState(null)
  const [loading, setLoading] = useState(true)
  const [statusMsg, setStatusMsg] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  // Network Import Dialog
  const [openUrlDialog, setOpenUrlDialog] = useState(false)
  const [importUrl, setImportUrl] = useState('')
  const [importingUrl, setImportingUrl] = useState(false)

  // Ping test
  const [pinging, setPinging] = useState(false)
  const [pingResults, setPingResults] = useState(null)

  // Hidden file input
  const fileInputRef = useRef(null)

  const loadSources = async () => {
    setLoading(true)
    setErrorMsg('')
    try {
      const res = await getSources()
      setSources(res.sources || [])
      setActiveMeta(res.active || null)
    } catch (err) {
      setErrorMsg('加载音源列表失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadSources()
  }, [])

  const handleActivate = async (id) => {
    try {
      const res = await activateSource(id)
      if (res.success) {
        setActiveMeta(res.meta)
        setSources(res.sources || [])
        setStatusMsg(`已成功切换激活音源: ${res.meta?.name}`)
      }
    } catch (err) {
      setErrorMsg('激活失败: ' + err.message)
    }
  }

  const handleDelete = async (id, name) => {
    if (!window.confirm(`确定要移除音源 “${name}” 吗？`)) return
    try {
      const res = await deleteSource(id)
      if (res.success) {
        setSources(res.sources || [])
        setStatusMsg(`已成功移除音源: ${name}`)
      }
    } catch (err) {
      setErrorMsg('删除失败: ' + err.message)
    }
  }

  const handleCheckUpdate = async (id, name) => {
    try {
      const res = await checkSourceUpdate(id)
      if (res.hasUpdate) {
        alert(`音源 “${name}” 发现新版本: ${res.newVersion}！`)
      } else {
        alert(`音源 “${name}” 当前已是最新版本 (${res.newVersion})`)
      }
    } catch (err) {
      alert('检查更新失败: ' + err.message)
    }
  }

  const handleImportFromUrl = async () => {
    if (!importUrl.trim()) return
    setImportingUrl(true)
    setErrorMsg('')
    try {
      const res = await importSourceFromUrl(importUrl.trim())
      if (res.success) {
        setOpenUrlDialog(false)
        setImportUrl('')
        setStatusMsg(`网络音源导入成功并已激活: ${res.meta?.name}`)
        setActiveMeta(res.meta)
        setSources(res.sources || [])
      }
    } catch (err) {
      setErrorMsg('网络导入失败: ' + (err.body || err.message))
    } finally {
      setImportingUrl(false)
    }
  }

  const handleFileUpload = (e) => {
    const file = e.target.files && e.target.files[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = async (event) => {
      const content = event.target.result
      try {
        const res = await uploadSource(file.name.replace(/\.js$/, ''), '本地文件导入', content)
        if (res.success) {
          setStatusMsg(`本地脚本导入成功并已激活: ${res.meta?.name}`)
          setActiveMeta(res.meta)
          setSources(res.sources || [])
        }
      } catch (err) {
        setErrorMsg('本地脚本导入失败: ' + (err.body || err.message))
      }
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  const handlePingTest = async () => {
    setPinging(true)
    setPingResults(null)
    try {
      const res = await pingSources()
      setPingResults(res.results || [])
    } catch (err) {
      setErrorMsg('连通性测试失败: ' + err.message)
    } finally {
      setPinging(false)
    }
  }

  return (
    <div className={classes.root}>
      {/* 头部标题与操作 */}
      <div className={classes.headerBox}>
        <Box display="flex" alignItems="center">
          <SettingsEthernetIcon style={{ marginRight: 8, color: '#10B981', fontSize: 32 }} />
          <div>
            <Typography variant="h5" style={{ fontWeight: 'bold' }}>
              网络音源管理
            </Typography>
            <Typography variant="body2" color="textSecondary">
              对齐洛雪 (LX Music) 自定义源原生规范，支持订阅链接同步与本地脚本导入
            </Typography>
          </div>
        </Box>

        <div className={classes.actionButtons}>
          <Button
            variant="contained"
            color="primary"
            startIcon={<CloudDownloadIcon />}
            onClick={() => {
              setErrorMsg('')
              setOpenUrlDialog(true)
            }}
            style={{ borderRadius: 8, textTransform: 'none' }}
          >
            网络导入 (订阅链接)
          </Button>

          <Button
            variant="outlined"
            color="primary"
            startIcon={<PublishIcon />}
            onClick={() => fileInputRef.current && fileInputRef.current.click()}
            style={{ borderRadius: 8, textTransform: 'none' }}
          >
            本地文件导入 (.js)
          </Button>
          <input
            type="file"
            accept=".js"
            ref={fileInputRef}
            style={{ display: 'none' }}
            onChange={handleFileUpload}
          />

          <Button
            variant="outlined"
            startIcon={pinging ? <CircularProgress size={16} /> : <SpeedIcon />}
            onClick={handlePingTest}
            disabled={pinging}
            style={{ borderRadius: 8, textTransform: 'none' }}
          >
            {pinging ? '测速中...' : '一键测速 (Ping)'}
          </Button>
        </div>
      </div>

      {/* 状态提醒通知 */}
      {statusMsg && (
        <Box mb={2} p={1.5} borderRadius={8} bgcolor="rgba(16, 185, 129, 0.15)" color="#10b981">
          <Typography variant="body2">{statusMsg}</Typography>
        </Box>
      )}
      {errorMsg && (
        <Box mb={2} p={1.5} borderRadius={8} bgcolor="rgba(239, 68, 68, 0.15)" color="#ef4444">
          <Typography variant="body2">{errorMsg}</Typography>
        </Box>
      )}

      {/* 连通性测试结果面板 */}
      {pingResults && (
        <div className={classes.pingCard}>
          <Box display="flex" alignItems="center" mb={1.5}>
            <SpeedIcon style={{ fontSize: 20, marginRight: 6, color: '#3B82F6' }} />
            <Typography variant="subtitle2" style={{ fontWeight: 'bold' }}>
              平台连通性与网络延迟 (Ping 结果)
            </Typography>
          </Box>
          <Box display="flex" gap={1.5} flexWrap="wrap">
            {pingResults.map((p) => {
              const isFast = p.delayMs < 600
              const isSlow = p.delayMs >= 600 && p.delayMs < 1500
              const isError = p.status === 'error'
              const color = isError ? '#EF4444' : isFast ? '#10B981' : '#F59E0B'
              return (
                <Chip
                  key={p.platform}
                  label={`${p.name}: ${isError ? '不可用' : `${p.delayMs}ms`}`}
                  style={{
                    backgroundColor: `${color}20`,
                    color: color,
                    fontWeight: 600,
                    border: `1px solid ${color}40`,
                  }}
                  size="small"
                />
              )
            })}
          </Box>
        </div>
      )}

      {/* 音源列表展示 */}
      {loading ? (
        <Box display="flex" justifyContent="center" p={6}>
          <CircularProgress />
        </Box>
      ) : sources.length === 0 ? (
        <Card className={classes.card}>
          <CardContent style={{ textAlign: 'center', padding: '40px 20px' }}>
            <Typography variant="body1" color="textSecondary">
              暂未导入任何音源脚本。请点击上方【网络导入】或【本地文件导入】添加洛雪自定义音源。
            </Typography>
          </CardContent>
        </Card>
      ) : (
        sources.map((item) => {
          const isActive = item.isActive || item.id === activeMeta?.id
          return (
            <Card key={item.id} className={`${classes.card} ${isActive ? classes.activeCard : ''}`}>
              <CardContent>
                {/* 顶部标题与激活标识 */}
                <div className={classes.cardHeader}>
                  <div className={classes.titleGroup}>
                    <Radio
                      checked={isActive}
                      onChange={() => handleActivate(item.id)}
                      value={item.id}
                      name="source-active-radio"
                      color="primary"
                      style={{ padding: 4 }}
                    />
                    <Typography variant="h6" style={{ fontWeight: 600 }}>
                      {item.name}
                    </Typography>
                    {item.version && (
                      <Chip
                        label={item.version.startsWith('v') ? item.version : `v${item.version}`}
                        size="small"
                        style={{
                          backgroundColor: '#8B5CF620',
                          color: '#8B5CF6',
                          fontWeight: 600,
                          fontSize: '0.75rem',
                        }}
                      />
                    )}
                    {isActive ? (
                      <Chip
                        icon={<CheckCircleIcon style={{ color: '#10B981' }} />}
                        label="当前激活"
                        size="small"
                        style={{
                          backgroundColor: '#10B98120',
                          color: '#10B981',
                          fontWeight: 'bold',
                        }}
                      />
                    ) : (
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => handleActivate(item.id)}
                        style={{ textTransform: 'none', padding: '1px 8px', fontSize: '0.75rem' }}
                      >
                        点击启用
                      </Button>
                    )}
                  </div>

                  {!isActive && (
                    <Tooltip title="移除此音源">
                      <IconButton
                        size="small"
                        onClick={() => handleDelete(item.id, item.name)}
                        style={{ color: '#9CA3AF' }}
                      >
                        <DeleteOutlineIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  )}
                </div>

                {/* 描述与作者信息 */}
                <Typography variant="body2" color="textSecondary" style={{ marginTop: 6 }}>
                  {item.description || '无详细描述'}
                </Typography>

                <Box display="flex" alignItems="center" gap={2} mt={1} color="text.secondary" fontSize="0.8rem">
                  <span>作者: {item.author || '网络热心发烧友'}</span>
                  {item.updatedAt && (
                    <span>更新于: {new Date(item.updatedAt).toLocaleDateString()}</span>
                  )}
                </Box>

                {/* 订阅链接 */}
                {item.subscriptionUrl && (
                  <Box display="flex" alignItems="center" gap={0.5} mt={1} fontSize="0.78rem" color="primary.main">
                    <LinkIcon fontSize="small" />
                    <span style={{ wordBreak: 'break-all' }}>{item.subscriptionUrl}</span>
                  </Box>
                )}

                {/* 支持平台彩色徽标 */}
                <div className={classes.platformChips}>
                  {(item.sources || ['wy', 'tx', 'kg', 'kw', 'mg']).map((src) => {
                    const info = PLATFORM_MAP[src] || { name: src.toUpperCase(), color: '#6B7280' }
                    return (
                      <Chip
                        key={src}
                        label={info.name}
                        size="small"
                        style={{
                          backgroundColor: `${info.color}15`,
                          color: info.color,
                          fontWeight: 600,
                          fontSize: '0.75rem',
                          border: `1px solid ${info.color}30`,
                        }}
                      />
                    )
                  })}
                  <Chip
                    label="最高 Hi-Res / FLAC"
                    size="small"
                    style={{
                      background: 'linear-gradient(135deg, #6366F1, #8B5CF6)',
                      color: '#ffffff',
                      fontWeight: 600,
                      fontSize: '0.72rem',
                    }}
                  />
                </div>

                {/* 底部功能条 */}
                <div className={classes.footerRow}>
                  <Typography variant="caption" color="textSecondary">
                    ID: {item.id}
                  </Typography>

                  <Box display="flex" gap={1}>
                    {item.subscriptionUrl && (
                      <Button
                        size="small"
                        startIcon={<RefreshIcon fontSize="small" />}
                        onClick={() => handleCheckUpdate(item.id, item.name)}
                        style={{ textTransform: 'none', fontSize: '0.78rem' }}
                      >
                        检查更新
                      </Button>
                    )}
                  </Box>
                </div>
              </CardContent>
            </Card>
          )
        })
      )}

      {/* 网络导入对话框 */}
      <Dialog open={openUrlDialog} onClose={() => !importingUrl && setOpenUrlDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>从网络导入音源 (订阅链接)</DialogTitle>
        <DialogContent>
          <DialogContentText style={{ fontSize: '0.9rem' }}>
            输入远程音源脚本直链 URL（如 GitHub Raw、Gitee 或第三方订阅源）。系统将自动拉取、校验语法并解析元数据。
          </DialogContentText>
          <TextField
            autoFocus
            margin="dense"
            label="音源脚本 URL (https://...)"
            type="url"
            fullWidth
            variant="outlined"
            value={importUrl}
            onChange={(e) => setImportUrl(e.target.value)}
            disabled={importingUrl}
            placeholder="https://example.com/source.js"
          />
        </DialogContent>
        <DialogActions style={{ padding: '12px 20px' }}>
          <Button onClick={() => setOpenUrlDialog(false)} disabled={importingUrl}>
            取消
          </Button>
          <Button
            onClick={handleImportFromUrl}
            color="primary"
            variant="contained"
            disabled={importingUrl || !importUrl.trim()}
          >
            {importingUrl ? <CircularProgress size={20} /> : '一键拉取并激活'}
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default SourceSettings

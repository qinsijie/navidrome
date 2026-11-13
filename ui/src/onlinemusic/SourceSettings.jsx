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
  Select,
  FormControl,
  InputLabel,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
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
import StorageIcon from '@material-ui/icons/Storage'
import ShareIcon from '@material-ui/icons/Share'
import InsertDriveFileIcon from '@material-ui/icons/InsertDriveFile'
import FileCopyIcon from '@material-ui/icons/FileCopy'
import {
  getSources,
  uploadSource,
  importSourceFromUrl,
  activateSource,
  deleteSource,
  checkSourceUpdate,
  pingSources,
  getOnlineCacheStats,
  setOnlineCacheConfig,
  clearOnlineCache,
  getSourceDownloadUrl,
  downloadSourceScript,
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
  qs: { name: '汽水音乐', color: '#06b6d4' },
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

  // Online audio cache config
  const [cacheStats, setCacheStats] = useState(null)
  const [cacheLimitMB, setCacheLimitMB] = useState(1024)
  const [savingCache, setSavingCache] = useState(false)
  const [clearingCache, setClearingCache] = useState(false)

  // Source share menu & fallback export dialog
  const [shareAnchorEl, setShareAnchorEl] = useState(null)
  const [shareTargetItem, setShareTargetItem] = useState(null)
  const [sharingFile, setSharingFile] = useState(false)
  const [exportDialogItem, setExportDialogItem] = useState(null)
  const [copyingScript, setCopyingScript] = useState(false)

  // Hidden file input
  const fileInputRef = useRef(null)

  const loadCacheStats = async () => {
    try {
      const res = await getOnlineCacheStats()
      if (res) {
        setCacheStats(res)
        if (res.maxMB) setCacheLimitMB(res.maxMB)
      }
    } catch {
      // ignore
    }
  }

  const loadSources = async () => {
    setLoading(true)
    setErrorMsg('')
    try {
      const [res] = await Promise.all([getSources(), loadCacheStats()])
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

  const handleSaveCacheLimit = async (nextMB) => {
    setSavingCache(true)
    setErrorMsg('')
    try {
      const res = await setOnlineCacheConfig(nextMB)
      if (res) {
        setCacheStats(res)
        setCacheLimitMB(res.maxMB || nextMB)
        setStatusMsg(`在线音频缓存上限已更新为 ${nextMB >= 1024 ? `${(nextMB / 1024).toFixed(nextMB % 1024 === 0 ? 0 : 1)} GB` : `${nextMB} MB`}`)
      }
    } catch (err) {
      setErrorMsg('保存缓存设置失败: ' + (err.body || err.message))
    } finally {
      setSavingCache(false)
    }
  }

  const handleClearCache = async () => {
    if (!window.confirm('确定要清空已缓存的在线歌曲文件吗？（正在播放的曲目不会受影响）')) {
      return
    }
    setClearingCache(true)
    setErrorMsg('')
    try {
      const res = await clearOnlineCache()
      if (res) {
        setCacheStats(res)
        setStatusMsg('在线音频缓存已清空')
      }
    } catch (err) {
      setErrorMsg('清空缓存失败: ' + (err.body || err.message))
    } finally {
      setClearingCache(false)
    }
  }

  const formatCacheBytes = (bytes) => {
    if (!bytes || bytes <= 0) return '0 MB'
    const mb = bytes / (1024 * 1024)
    if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`
    return `${mb.toFixed(1)} MB`
  }

  const copyToClipboardCompat = async (text) => {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      try {
        await navigator.clipboard.writeText(text)
        return true
      } catch {
        // fallback below
      }
    }
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      return ok
    } catch {
      return false
    }
  }

  const handleShareLink = async (item) => {
    if (!item?.subscriptionUrl) return
    const title = item.name || '落雪音源'
    const url = item.subscriptionUrl
    if (typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
      try {
        await navigator.share({
          title: `分享音源：${title}`,
          text: `${title} 音源订阅链接`,
          url,
        })
        setStatusMsg(`已调起系统分享：${title}`)
        return
      } catch (err) {
        if (err && err.name === 'AbortError') return
      }
    }
    const copied = await copyToClipboardCompat(url)
    if (copied) {
      setStatusMsg(`已复制音源链接（${title}），可直接粘贴发送到 QQ / 微信`)
    } else {
      setErrorMsg(`自动复制失败，请手动复制链接：${url}`)
    }
  }

  const triggerDirectHttpDownload = (item) => {
    const directUrl = getSourceDownloadUrl(item.id)
    const rawName = (item.name || item.id || 'source').trim()
    const safeName = rawName.replace(/[/\\:*?"<>|]/g, '_') || item.id
    const fileName = safeName.endsWith('.js') ? safeName : `${safeName}.js`

    const a = document.createElement('a')
    a.href = directUrl
    a.download = fileName
    a.style.display = 'none'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    return { directUrl, fileName }
  }

  const handleShareFile = async (item) => {
    if (!item?.id) return
    setErrorMsg('')

    // Tier 1: Modern mobile browsers supporting native File sharing
    if (
      typeof navigator !== 'undefined' &&
      typeof navigator.canShare === 'function' &&
      typeof navigator.share === 'function'
    ) {
      setSharingFile(true)
      try {
        const { blob, fileName } = await downloadSourceScript(item.id)
        const jsFile = new File([blob], fileName, {
          type: 'application/javascript',
        })
        if (navigator.canShare({ files: [jsFile] })) {
          try {
            await navigator.share({
              title: item.name || fileName,
              text: `分享音源脚本：${item.name || fileName}`,
              files: [jsFile],
            })
            setStatusMsg(`已调起系统文件分享：${fileName}`)
            return
          } catch (err) {
            if (err && err.name === 'AbortError') return
          }
        }
      } catch (err) {
        // Fall through to Tier 2 + Tier 3
      } finally {
        setSharingFile(false)
      }
    }

    // Tier 2: Synchronously trigger real HTTP download (preserves user gesture)
    const { fileName } = triggerDirectHttpDownload(item)
    setStatusMsg(`已触发文件下载：${fileName}`)

    // Tier 3: Open fallback dialog for restricted WebViews (e.g. fnOS App)
    setExportDialogItem(item)
  }

  const handleCopyScriptSource = async (item) => {
    if (!item?.id) return
    setCopyingScript(true)
    try {
      const { blob } = await downloadSourceScript(item.id)
      const text = await blob.text()
      const ok = await copyToClipboardCompat(text)
      if (ok) {
        setStatusMsg(`已复制「${item.name || item.id}」全部脚本源码，可粘贴保存为 .js 文件或转发`)
        setExportDialogItem(null)
      } else {
        setErrorMsg('自动复制失败，请尝试复制下方下载地址到手机浏览器下载')
      }
    } catch (err) {
      setErrorMsg('读取脚本源码失败: ' + err.message)
    } finally {
      setCopyingScript(false)
    }
  }

  const handleCopyDownloadUrl = async (item) => {
    if (!item?.id) return
    const relUrl = getSourceDownloadUrl(item.id)
    let fullUrl = relUrl
    try {
      fullUrl = new URL(relUrl, window.location.origin).href
    } catch {
      // keep relative
    }
    const ok = await copyToClipboardCompat(fullUrl)
    if (ok) {
      setStatusMsg('已复制脚本下载地址，请粘贴到手机浏览器打开即可下载 .js 文件')
      setExportDialogItem(null)
    } else {
      setErrorMsg('自动复制失败，请手动复制地址: ' + fullUrl)
    }
  }

  const handleOpenShare = (item, e) => {
    if (item.subscriptionUrl) {
      setShareAnchorEl(e.currentTarget)
      setShareTargetItem(item)
    } else {
      handleShareFile(item)
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

                  <Box display="flex" style={{ gap: 8 }}>
                    <Button
                      size="small"
                      startIcon={
                        sharingFile ? (
                          <CircularProgress size={14} />
                        ) : (
                          <ShareIcon fontSize="small" />
                        )
                      }
                      disabled={sharingFile}
                      onClick={(e) => handleOpenShare(item, e)}
                      style={{ textTransform: 'none', fontSize: '0.78rem' }}
                    >
                      分享
                    </Button>
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

      {/* 链接音源分享选择菜单 */}
      <Menu
        anchorEl={shareAnchorEl}
        open={Boolean(shareAnchorEl)}
        onClose={() => {
          setShareAnchorEl(null)
          setShareTargetItem(null)
        }}
      >
        <MenuItem
          onClick={() => {
            const target = shareTargetItem
            setShareAnchorEl(null)
            setShareTargetItem(null)
            if (target) handleShareLink(target)
          }}
        >
          <ListItemIcon style={{ minWidth: 34 }}>
            <LinkIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="分享源链接" />
        </MenuItem>
        <MenuItem
          onClick={() => {
            const target = shareTargetItem
            setShareAnchorEl(null)
            setShareTargetItem(null)
            if (target) handleShareFile(target)
          }}
        >
          <ListItemIcon style={{ minWidth: 34 }}>
            <InsertDriveFileIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="分享 JS 文件" />
        </MenuItem>
      </Menu>

      {/* 在线音频缓存设置卡片 */}
      <Card className={classes.card}>
        <CardContent>
          <Box
            display="flex"
            alignItems="center"
            justifyContent="space-between"
            flexWrap="wrap"
            style={{ gap: 12 }}
          >
            <Box display="flex" alignItems="center" style={{ gap: 10 }}>
              <StorageIcon color="primary" />
              <Box>
                <Typography variant="subtitle1" style={{ fontWeight: 600 }}>
                  在线音频缓存（边听边存）
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  {cacheStats
                    ? `已缓存 ${formatCacheBytes(cacheStats.usedBytes)} / ${formatCacheBytes(cacheStats.maxBytes)}（共 ${cacheStats.fileCount || 0} 首）`
                    : '正在读取缓存状态...'}
                </Typography>
              </Box>
            </Box>

            <Box display="flex" alignItems="center" flexWrap="wrap" style={{ gap: 12 }}>
              <FormControl variant="outlined" size="small" style={{ minWidth: 160 }}>
                <InputLabel id="online-cache-limit-label">缓存空间上限</InputLabel>
                <Select
                  labelId="online-cache-limit-label"
                  label="缓存空间上限"
                  value={cacheLimitMB}
                  disabled={savingCache}
                  onChange={(e) => handleSaveCacheLimit(Number(e.target.value))}
                >
                  <MenuItem value={512}>512 MB</MenuItem>
                  <MenuItem value={1024}>1 GB（默认）</MenuItem>
                  <MenuItem value={2048}>2 GB</MenuItem>
                  <MenuItem value={3072}>3 GB</MenuItem>
                  <MenuItem value={5120}>5 GB</MenuItem>
                  <MenuItem value={10240}>10 GB</MenuItem>
                </Select>
              </FormControl>

              <Button
                variant="outlined"
                color="secondary"
                size="small"
                startIcon={
                  clearingCache ? (
                    <CircularProgress size={16} />
                  ) : (
                    <DeleteOutlineIcon fontSize="small" />
                  )
                }
                disabled={clearingCache || !cacheStats?.fileCount}
                onClick={handleClearCache}
                style={{ height: 40, textTransform: 'none' }}
              >
                清空缓存
              </Button>
            </Box>
          </Box>
        </CardContent>
      </Card>

      {/* 受限 WebView 导出/分享 JS 文件兜底弹窗 */}
      <Dialog
        open={Boolean(exportDialogItem)}
        onClose={() => !copyingScript && setExportDialogItem(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>导出音源 JS 文件</DialogTitle>
        <DialogContent>
          <DialogContentText style={{ fontSize: '0.9rem', marginBottom: 16 }}>
            已尝试触发浏览器下载「{exportDialogItem?.name || exportDialogItem?.id}.js」。
            若在飞牛 App 内置 WebView 中被宿主拦截未弹出保存，可使用以下两种方式导出：
          </DialogContentText>
          <Box display="flex" flexDirection="column" style={{ gap: 10 }}>
            <Button
              variant="contained"
              color="primary"
              fullWidth
              disabled={copyingScript}
              startIcon={
                copyingScript ? (
                  <CircularProgress size={16} color="inherit" />
                ) : (
                  <FileCopyIcon />
                )
              }
              onClick={() => handleCopyScriptSource(exportDialogItem)}
              style={{ textTransform: 'none' }}
            >
              一键复制脚本源码
            </Button>
            <Button
              variant="outlined"
              color="primary"
              fullWidth
              startIcon={<LinkIcon />}
              onClick={() => handleCopyDownloadUrl(exportDialogItem)}
              style={{ textTransform: 'none' }}
            >
              复制下载地址（去手机浏览器粘贴下载）
            </Button>
          </Box>
        </DialogContent>
        <DialogActions style={{ padding: '8px 20px 14px' }}>
          <Button
            onClick={() => setExportDialogItem(null)}
            disabled={copyingScript}
          >
            关闭
          </Button>
        </DialogActions>
      </Dialog>

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

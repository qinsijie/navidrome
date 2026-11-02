import React, { useState, useEffect } from 'react'
import {
  Card,
  CardContent,
  Typography,
  Button,
  TextField,
  Chip,
  Box,
  CircularProgress,
  Divider,
  makeStyles,
} from '@material-ui/core'
import SettingsEthernetIcon from '@material-ui/icons/SettingsEthernet'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import PublishIcon from '@material-ui/icons/Publish'
import { getSources, uploadSource } from './api'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    maxWidth: 1000,
    margin: '0 auto',
  },
  card: {
    marginBottom: theme.spacing(3),
  },
  statusHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    marginBottom: theme.spacing(2),
  },
  chipContainer: {
    display: 'flex',
    gap: theme.spacing(1),
    marginTop: theme.spacing(1),
    flexWrap: 'wrap',
  },
  textArea: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(2),
    fontFamily: 'monospace',
    fontSize: '0.85rem',
  },
  buttonContainer: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: theme.spacing(2),
  },
}))

export const SourceSettings = () => {
  const classes = useStyles()
  const [meta, setMeta] = useState(null)
  const [loading, setLoading] = useState(true)
  const [scriptContent, setScriptContent] = useState('')
  const [scriptName, setScriptName] = useState('Custom Source Script')
  const [saving, setSaving] = useState(false)
  const [statusMsg, setStatusMsg] = useState('')

  const loadSourceStatus = () => {
    setLoading(true)
    getSources()
      .then((res) => {
        setMeta(res.active || null)
      })
      .catch((err) => console.error(err))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadSourceStatus()
  }, [])

  const handleUpload = async () => {
    if (!scriptContent.trim()) return
    setSaving(true)
    setStatusMsg('')
    try {
      const res = await uploadSource(scriptName, 'User uploaded script', scriptContent)
      if (res.success) {
        setStatusMsg('音源脚本更新并热生效成功！')
        setMeta(res.meta)
        setScriptContent('')
      }
    } catch (err) {
      setStatusMsg('脚本生效失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className={classes.root}>
      <Box display="flex" alignItems="center" mb={3}>
        <SettingsEthernetIcon style={{ marginRight: 8, color: '#10B981', fontSize: 32 }} />
        <Typography variant="h5">网络音源脚本管理</Typography>
      </Box>

      <Card className={classes.card}>
        <CardContent>
          <div className={classes.statusHeader}>
            <CheckCircleIcon style={{ color: meta ? '#10B981' : '#9CA3AF' }} />
            <Typography variant="h6">当前运行音源引擎</Typography>
          </div>

          {loading ? (
            <CircularProgress size={30} />
          ) : meta ? (
            <div>
              <Typography variant="body1">
                <strong>名称：</strong> {meta.name || '默认音源'}
              </Typography>
              <Typography variant="body2" color="textSecondary">
                <strong>版本哈希：</strong> {meta.version}
              </Typography>
              <Typography variant="body2" color="textSecondary">
                <strong>最后更新：</strong> {new Date(meta.updatedAt).toLocaleString()}
              </Typography>
              <div className={classes.chipContainer}>
                {(meta.sources || ['wy', 'tx', 'kg', 'kw', 'mg']).map((s) => (
                  <Chip
                    key={s}
                    label={s.toUpperCase()}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                ))}
              </div>
            </div>
          ) : (
            <Typography variant="body2" color="textSecondary">
              未加载激活音源脚本，请在下方上传或导入音源 JS 脚本。
            </Typography>
          )}
        </CardContent>
      </Card>

      <Card className={classes.card}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            导入 / 更新音源脚本
          </Typography>
          <Typography variant="body2" color="textSecondary">
            支持标准 LX Music (洛雪音乐) 自定义源脚本规范，支持纯 Goja 沙箱沙箱原生热载入。
          </Typography>

          <TextField
            fullWidth
            label="音源脚本名称"
            variant="outlined"
            margin="dense"
            value={scriptName}
            onChange={(e) => setScriptName(e.target.value)}
          />

          <TextField
            className={classes.textArea}
            fullWidth
            multiline
            rows={12}
            variant="outlined"
            placeholder="// 在此粘贴音源脚本内容 (JavaScript)..."
            value={scriptContent}
            onChange={(e) => setScriptContent(e.target.value)}
          />

          {statusMsg && (
            <Box mb={2}>
              <Typography
                variant="body2"
                style={{ color: statusMsg.includes('成功') ? '#10B981' : '#EF4444' }}
              >
                {statusMsg}
              </Typography>
            </Box>
          )}

          <div className={classes.buttonContainer}>
            <Button
              variant="contained"
              color="primary"
              startIcon={<PublishIcon />}
              onClick={handleUpload}
              disabled={saving || !scriptContent.trim()}
            >
              {saving ? '正在验证并激活...' : '立即保存并激活'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export default SourceSettings

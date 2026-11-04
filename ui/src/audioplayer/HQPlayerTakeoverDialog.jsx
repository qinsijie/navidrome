import React from 'react'
import PropTypes from 'prop-types'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Typography,
} from '@material-ui/core'
import WarningIcon from '@material-ui/icons/Warning'

export const HQPlayerTakeoverDialog = ({ open, onClose, onConfirm, occupiedBy }) => {
  const userName = occupiedBy?.userName || occupiedBy?.userId || '其他用户'
  const trackTitle = occupiedBy?.trackTitle || occupiedBy?.track?.title || '外部音频流'

  return (
    <Dialog
      open={open}
      onClose={onClose}
      aria-labelledby="hqplayer-takeover-title"
      aria-describedby="hqplayer-takeover-description"
      maxWidth="xs"
      fullWidth
    >
      <DialogTitle id="hqplayer-takeover-title" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <WarningIcon style={{ color: '#F59E0B' }} />
        <span>HQPlayer 设备占用提示</span>
      </DialogTitle>
      <DialogContent>
        <DialogContentText id="hqplayer-takeover-description" style={{ color: 'inherit' }}>
          HQPlayer 当前正被用户 <strong style={{ color: '#3B82F6' }}>[{userName}]</strong> 使用
          （正在播放：<strong style={{ color: '#10B981' }}>[{trackTitle}]</strong>），是否切换接管？
        </DialogContentText>
        <Typography variant="caption" color="textSecondary" style={{ display: 'block', marginTop: 8 }}>
          确认接管后，服务端将执行 500ms 硬件排空安全时序，释放原通道并切入当前推流。
        </Typography>
      </DialogContent>
      <DialogActions style={{ padding: '12px 16px' }}>
        <Button onClick={onClose} color="default">
          取消
        </Button>
        <Button onClick={onConfirm} color="primary" variant="contained" autoFocus>
          确认接管
        </Button>
      </DialogActions>
    </Dialog>
  )
}

HQPlayerTakeoverDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onConfirm: PropTypes.func.isRequired,
  occupiedBy: PropTypes.object,
}

export default HQPlayerTakeoverDialog

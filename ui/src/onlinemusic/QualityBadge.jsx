import React from 'react'
import { makeStyles } from '@material-ui/core'

const useStyles = makeStyles((theme) => ({
  badge: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '2px 6px',
    borderRadius: '4px',
    fontSize: '0.68rem',
    fontWeight: 700,
    letterSpacing: '0.5px',
    textTransform: 'uppercase',
    userSelect: 'none',
    lineHeight: 1.2,
  },
  hires: {
    background: 'linear-gradient(135deg, #7928CA, #4338CA)',
    color: '#FFFFFF',
    boxShadow: '0 1px 4px rgba(121, 40, 202, 0.4)',
    border: '1px solid rgba(255, 255, 255, 0.2)',
  },
  flac: {
    background: 'rgba(16, 185, 129, 0.15)',
    color: '#10B981',
    border: '1px solid rgba(16, 185, 129, 0.35)',
  },
  high320: {
    background: 'rgba(245, 158, 11, 0.15)',
    color: '#F59E0B',
    border: '1px solid rgba(245, 158, 11, 0.35)',
  },
  standard128: {
    background: 'rgba(156, 163, 175, 0.15)',
    color: '#9CA3AF',
    border: '1px solid rgba(156, 163, 175, 0.25)',
  },
}))

export const QualityBadge = ({ quality }) => {
  const classes = useStyles()
  const q = (quality || 'flac').toLowerCase()

  if (q.includes('24') || q.includes('hires')) {
    return <span className={`${classes.badge} ${classes.hires}`}>Hi-Res</span>
  }
  if (q.includes('flac') || q.includes('sq')) {
    return <span className={`${classes.badge} ${classes.flac}`}>FLAC</span>
  }
  if (q.includes('320') || q.includes('hq')) {
    return <span className={`${classes.badge} ${classes.high320}`}>320K</span>
  }
  return <span className={`${classes.badge} ${classes.standard128}`}>128K</span>
}

export default QualityBadge

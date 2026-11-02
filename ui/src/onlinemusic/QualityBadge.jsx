import React from 'react'
import { makeStyles } from '@material-ui/core'

const useStyles = makeStyles((theme) => ({
  badge: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '1px 5px',
    borderRadius: '3px',
    fontSize: '0.65rem',
    fontWeight: 700,
    letterSpacing: '0.3px',
    textTransform: 'uppercase',
    userSelect: 'none',
    lineHeight: 1.2,
    marginRight: '6px',
  },
  hires: {
    background: 'rgba(139, 92, 246, 0.15)',
    color: '#8B5CF6',
    border: '1px solid rgba(139, 92, 246, 0.4)',
  },
  sq: {
    background: 'rgba(16, 185, 129, 0.15)',
    color: '#10B981',
    border: '1px solid rgba(16, 185, 129, 0.4)',
  },
  hq: {
    background: 'rgba(245, 158, 11, 0.15)',
    color: '#F59E0B',
    border: '1px solid rgba(245, 158, 11, 0.4)',
  },
  standard: {
    background: 'rgba(156, 163, 175, 0.15)',
    color: '#9CA3AF',
    border: '1px solid rgba(156, 163, 175, 0.3)',
  },
  sourceBadge: {
    background: 'rgba(59, 130, 246, 0.12)',
    color: '#3B82F6',
    border: '1px solid rgba(59, 130, 246, 0.3)',
  },
}))

export const QualityBadge = ({ quality }) => {
  const classes = useStyles()
  const q = (quality || 'flac').toLowerCase()

  if (q.includes('24') || q.includes('hires')) {
    return <span className={`${classes.badge} ${classes.hires}`}>24bit</span>
  }
  if (q.includes('flac') || q.includes('sq')) {
    return <span className={`${classes.badge} ${classes.sq}`}>SQ</span>
  }
  if (q.includes('320') || q.includes('hq')) {
    return <span className={`${classes.badge} ${classes.hq}`}>HQ</span>
  }
  return <span className={`${classes.badge} ${classes.standard}`}>128K</span>
}

export const SourceBadge = ({ source }) => {
  const classes = useStyles()
  const s = (source || '').toLowerCase()
  return <span className={`${classes.badge} ${classes.sourceBadge}`}>{s || 'net'}</span>
}

export default QualityBadge

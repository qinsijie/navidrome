export const isValidTrackId = (id) => {
  if (id === undefined || id === null) return false
  const s = String(id).trim()
  return s !== '' && s !== 'undefined' && s !== 'null'
}

export const resolveTrackId = (item) => {
  if (!item || typeof item !== 'object') return ''
  const candidates = [
    item.id,
    item.ID,
    item.mediaFileId,
    item.trackId,
    item.song?.id,
    item.song?.ID,
    item.song?.mediaFileId,
    item.song?.trackId,
  ]
  for (const cand of candidates) {
    if (isValidTrackId(cand)) {
      return String(cand).trim()
    }
  }
  return ''
}

export const buildRemoteOutputTrack = (item) => {
  const tid = resolveTrackId(item)
  if (!isValidTrackId(tid)) return null
  const streamUrl = `/api/output/stream/${tid}`
  if (streamUrl.includes('undefined.wav') || streamUrl.endsWith('/undefined')) {
    return null
  }
  return {
    trackId: tid,
    title: item.song?.title || item.name || item.title || '',
    artist: item.song?.artist || item.singer || item.artist || '',
    album: item.song?.album || item.album || '',
    duration: item.duration || item.song?.duration || 0,
    streamUrl,
  }
}

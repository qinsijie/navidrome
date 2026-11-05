import React from 'react'
import { useMediaQuery } from '@material-ui/core'
import { Link } from 'react-router-dom'
import clsx from 'clsx'
import { QualityInfo } from '../common'
import { decisionService } from '../transcode'
import useStyle from './styles'
import { useDrag } from 'react-dnd'
import { DraggableTypes } from '../consts'

const AudioTitle = React.memo(({ audioInfo, gainInfo, isMobile }) => {
  const classes = useStyle()
  const className = classes.audioTitle
  const isDesktop = useMediaQuery('(min-width:810px)')

  const song = audioInfo?.song
  const songId = song?.id || audioInfo?.trackId
  const [, dragSongRef] = useDrag(
    () => ({
      type: DraggableTypes.SONG,
      item: { ids: songId ? [songId] : [] },
      options: { dropEffect: 'copy' },
    }),
    [songId],
  )

  if (!song || (!song.id && !song.title)) {
    return (
      <span className={classes.songInfo}>
        <span className={classes.songTitle}>{audioInfo?.name || 'Navidrome 就绪'}</span>
        {audioInfo?.singer && <span className={classes.songAlbum}> - {audioInfo.singer}</span>}
      </span>
    )
  }

  const qi = {
    suffix: song.suffix,
    bitRate: song.bitRate,
    rgAlbumGain: song.rgAlbumGain,
    rgAlbumPeak: song.rgAlbumPeak,
    rgTrackGain: song.rgTrackGain,
    rgTrackPeak: song.rgTrackPeak,
  }

  const trackId = audioInfo?.trackId || song.id
  const decision = trackId ? decisionService.getCachedDecision(trackId) : null
  const transcodeProps = decision
    ? {
        transcodeStream: decision.transcodeStream || null,
        isDirectPlay: decision.canDirectPlay,
      }
    : {}

  const subtitle = song.tags?.['subtitle']
  const displayTitle = (song.title || audioInfo?.name || '未知曲目') + (subtitle ? ` (${subtitle})` : '')
  const displayArtist = song.artist || audioInfo?.singer || ''
  const displayAlbum = song.album || ''

  const linkTo = audioInfo?.isRadio
    ? `/radio/${trackId}/show`
    : song.playlistId
      ? `/playlist/${song.playlistId}/show`
      : song.albumId
        ? `/album/${song.albumId}/show`
        : '#'

  const titleContent = (
    <>
      <span>
        <span className={clsx(classes.songTitle, 'songTitle')}>{displayTitle}</span>
        {isDesktop && (
          <QualityInfo
            record={qi}
            className={classes.qualityInfo}
            {...gainInfo}
            {...transcodeProps}
          />
        )}
      </span>
      {isMobile ? (
        <>
          {displayArtist && (
            <span className={classes.songInfo}>
              <span className={'songArtist'}>{displayArtist}</span>
            </span>
          )}
          {displayAlbum && (
            <span className={clsx(classes.songInfo, classes.songAlbum)}>
              <span className={'songAlbum'}>{displayAlbum}</span>
              {song.year ? ` - ${song.year}` : ''}
            </span>
          )}
        </>
      ) : (
        <span className={classes.songInfo}>
          {displayArtist && <span className={'songArtist'}>{displayArtist}</span>}
          {displayArtist && displayAlbum && ' - '}
          {displayAlbum && <span className={'songAlbum'}>{displayAlbum}</span>}
          {song.year ? ` - ${song.year}` : ''}
        </span>
      )}
    </>
  )

  if (linkTo === '#') {
    return (
      <span className={className} ref={dragSongRef}>
        {titleContent}
      </span>
    )
  }

  return (
    <Link to={linkTo} className={className} ref={dragSongRef}>
      {titleContent}
    </Link>
  )
})

AudioTitle.displayName = 'AudioTitle'

export default AudioTitle

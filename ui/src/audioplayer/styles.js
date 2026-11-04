import { makeStyles } from '@material-ui/core/styles'

const useStyle = makeStyles(
  (theme) => ({
    audioTitle: {
      textDecoration: 'none',
      color: theme.palette.primary.dark,
    },
    songTitle: {
      fontWeight: 'bold',
      '&:hover + $qualityInfo': {
        opacity: 1,
      },
    },
    songInfo: {
      display: 'block',
      marginTop: '2px',
    },
    songAlbum: {
      fontStyle: 'italic',
      fontSize: 'smaller',
    },
    qualityInfo: {
      display: 'inline-flex',
      marginLeft: '8px',
      verticalAlign: 'middle',
      opacity: '1 !important',
    },
    player: {
      display: (props) => (props.visible ? 'block' : 'none'),
      '& .music-player': {
        display: 'none !important',
      },
      '& .music-player-controller': {
        display: 'none !important',
      },
      '& .destroy-btn': {
        display: 'none !important',
      },
      '& .hide-panel': {
        display: 'none !important',
      },
      '& .music-player-panel, & .react-jinke-music-player-main .music-player-panel, & .react-jinke-music-player-main.light-theme .music-player-panel, & .react-jinke-music-player-main.dark-theme .music-player-panel': {
        position: 'fixed !important',
        bottom: '0 !important',
        left: '0 !important',
        width: '100vw !important',
        height: '72px !important',
        zIndex: '1300 !important',
        paddingBottom: 'env(safe-area-inset-bottom, 0px) !important',
        boxSizing: 'content-box !important',
        boxShadow:
          theme.palette.type === 'dark'
            ? '0 -4px 24px rgba(0, 0, 0, 0.35) !important'
            : '0 -4px 24px rgba(0, 0, 0, 0.08) !important',
        backgroundColor:
          theme.palette.type === 'dark'
            ? 'rgba(18, 18, 22, 0.22) !important'
            : 'rgba(255, 255, 255, 0.25) !important',
        backdropFilter: 'blur(24px) saturate(200%) !important',
        WebkitBackdropFilter: 'blur(24px) saturate(200%) !important',
        borderTop:
          theme.palette.type === 'dark'
            ? '1px solid rgba(255, 255, 255, 0.08) !important'
            : '1px solid rgba(0, 0, 0, 0.06) !important',
        color: `${theme.palette.text.primary} !important`,
        transition: 'background-color 0.3s ease, border-color 0.3s ease !important',
        transform: 'none !important',
      },
      '& .music-player-panel .panel-content': {
        height: '72px !important',
        display: 'flex !important',
        alignItems: 'center !important',
        justifyContent: 'space-between !important',
        padding: '0 24px !important',
        boxSizing: 'border-box !important',
        backgroundColor: 'transparent !important',
      },
      '& .music-player-panel .panel-content .img-content': {
        width: '48px !important',
        height: '48px !important',
        minWidth: '48px !important',
        borderRadius: '6px !important',
        marginRight: '12px !important',
      },
      '& .music-player-panel .panel-content div.img-rotate': {
        width: '48px !important',
        height: '48px !important',
        minWidth: '48px !important',
        borderRadius: '6px !important',
        animation: 'none !important',
        backgroundSize: 'cover !important',
        backgroundPosition: 'center !important',
      },
      '& .music-player-panel .panel-content .progress-bar-content': {
        display: 'flex !important',
        flexDirection: 'column !important',
        justifyContent: 'center !important',
        flex: '1 1 auto !important',
        padding: '0 20px !important',
        maxWidth: 'calc(100vw - 360px)',
      },
      '& .music-player-panel .panel-content .player-content': {
        display: 'flex !important',
        alignItems: 'center !important',
        flexShrink: 0,
      },
      '& .music-player-panel .panel-content .player-content .group': {
        margin: '0 6px !important',
      },
      '@media screen and (max-width:768px)': {
        '& .music-player-panel': {
          height: '60px !important',
          paddingBottom: 'env(safe-area-inset-bottom, 0px) !important',
          overflow: 'visible !important',
        },
        '& .music-player-panel .panel-content': {
          height: '60px !important',
          padding: '0 10px !important',
          position: 'relative !important',
          display: 'flex !important',
          alignItems: 'center !important',
          justifyContent: 'space-between !important',
        },
        '& .music-player-panel .progress-bar-content .progress-bar': {
          position: 'absolute !important',
          top: '0 !important',
          left: '0 !important',
          right: '0 !important',
          width: '100% !important',
          height: '3px !important',
          padding: '0 !important',
          margin: '0 !important',
          zIndex: '20 !important',
        },
        '& .music-player-panel .progress-bar .current-time, & .music-player-panel .progress-bar .progress-load-bar, & .music-player-panel .audio-time':
          {
            display: 'none !important',
          },
        '& .music-player-panel .progress-bar .rc-slider': {
          padding: '0 !important',
          margin: '0 !important',
          height: '3px !important',
        },
        '& .music-player-panel .progress-bar .rc-slider-rail, & .music-player-panel .progress-bar .rc-slider-track':
          {
            height: '3px !important',
            borderRadius: '0 !important',
          },
        '& .music-player-panel .progress-bar .rc-slider-handle': {
          display: 'none !important',
        },
        '& .music-player-panel .panel-content .img-content, & .music-player-panel .panel-content div.img-rotate':
          {
            width: '38px !important',
            height: '38px !important',
            minWidth: '38px !important',
            marginRight: '8px !important',
            borderRadius: '4px !important',
          },
        '& .music-player-panel .panel-content .progress-bar-content': {
          padding: '0 4px !important',
          margin: '0 !important',
          maxWidth: 'calc(100vw - 200px) !important',
          flex: '1 1 auto !important',
          minWidth: '0 !important',
        },
        '& .music-player-panel .panel-content .progress-bar-content .audio-title':
          {
            fontSize: '13px !important',
            whiteSpace: 'nowrap !important',
            overflow: 'hidden !important',
            textOverflow: 'ellipsis !important',
            display: 'block !important',
            lineHeight: '1.3 !important',
          },
        '& .music-player-panel .panel-content .progress-bar-content .songInfo': {
          fontSize: '11px !important',
          whiteSpace: 'nowrap !important',
          overflow: 'hidden !important',
          textOverflow: 'ellipsis !important',
          display: 'block !important',
          lineHeight: '1.2 !important',
          opacity: '0.75 !important',
        },
        '& .music-player-panel .panel-content .progress-bar-content .qualityInfo':
          {
            display: 'none !important',
          },
        '& .music-player-panel .panel-content .player-content': {
          display: 'flex !important',
          alignItems: 'center !important',
          justifyContent: 'flex-end !important',
          flexShrink: '0 !important',
        },
        '& .music-player-panel .panel-content .player-content .group': {
          margin: '0 2px !important',
        },
        '& .sound-operation, & .play-sounds, & .loop-btn, & .play-mode-title, & .lyric-btn':
          {
            display: 'none !important',
          },
        '& [data-testid="save-queue-button"], & button[aria-label*="love"], & .mobileListItem':
          {
            display: 'none !important',
          },
        '& .music-player-panel .panel-content .player-content .prev-audio, & .music-player-panel .panel-content .player-content .next-audio':
          {
            fontSize: '18px !important',
          },
        '& .music-player-panel .panel-content .player-content .play-btn': {
          fontSize: '26px !important',
          margin: '0 4px !important',
        },
        '& .music-player-panel .panel-content .player-content .audio-lists-btn':
          {
            display: 'flex !important',
            alignItems: 'center !important',
            marginLeft: '4px !important',
            fontSize: '20px !important',
          },
      },
      '@media screen and (max-width:810px)': {
        '& .sound-operation': {
          display: 'none',
        },
      },
      '@media (prefers-reduced-motion)': {
        '& .music-player-panel .panel-content div.img-rotate': {
          animation: 'none',
        },
      },
      '& .progress-bar-content': {
        display: 'flex',
        flexDirection: 'column',
      },
      '& .play-mode-title': {
        'pointer-events': 'none',
      },
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover':
        {
          // Customize mobile player when cover animation is disabled
          borderRadius: (props) => !props.enableCoverAnimation && '0',
          width: (props) => !props.enableCoverAnimation && '85%',
          maxWidth: (props) => !props.enableCoverAnimation && '600px',
          height: (props) => !props.enableCoverAnimation && 'auto',
          // Fix cover display when image is not square
          aspectRatio: '1/1',
          display: 'flex',
        },
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover img.cover':
        {
          animationDuration: (props) => !props.enableCoverAnimation && '0s',
          objectFit: 'contain', // Fix cover display when image is not square
        },
      // Hide old singer display
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-singer':
        {
          display: 'none',
        },
      // Hide extra whitespace from switch div
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-switch':
        {
          display: 'none',
        },
      '& .music-player-panel .panel-content .progress-bar-content section.audio-main':
        {
          display: (props) => (props.isRadio ? 'none' : 'inline-flex'),
        },
      '& .nd-stream-status-pill': {
        display: 'inline-flex !important',
        alignItems: 'center !important',
        gap: '6px !important',
        padding: '2px 8px !important',
        marginLeft: '8px !important',
        borderRadius: '12px !important',
        background: 'rgba(255, 255, 255, 0.06) !important',
        border: '1px solid rgba(255, 255, 255, 0.08) !important',
        backdropFilter: 'blur(8px) !important',
        verticalAlign: 'middle !important',
        userSelect: 'none !important',
      },
      '& .nd-stream-status-dot': {
        width: '6px !important',
        height: '6px !important',
        borderRadius: '50% !important',
        background: '#00ADB5 !important',
        boxShadow: '0 0 6px rgba(0, 173, 181, 0.5) !important',
        flexShrink: '0 !important',
      },
      '& .nd-stream-status-text': {
        fontSize: '11px !important',
        color: 'rgba(255, 255, 255, 0.65) !important',
        whiteSpace: 'nowrap !important',
        lineHeight: '1.2 !important',
      },
      '& .music-player-panel .progress-bar .rc-slider-rail': {
        height: '3px !important',
        backgroundColor:
          theme.palette.type === 'dark'
            ? 'rgba(255, 255, 255, 0.12) !important'
            : 'rgba(0, 0, 0, 0.10) !important',
      },
      '& .music-player-panel .progress-bar .rc-slider-track': {
        height: '3px !important',
        backgroundColor: '#00ADB5 !important',
      },
      '& .music-player-panel .progress-bar .rc-slider-handle': {
        width: '8px !important',
        height: '8px !important',
        marginTop: '-2.5px !important',
        backgroundColor: '#00ADB5 !important',
        border: 'none !important',
        boxShadow: '0 0 6px rgba(0, 173, 181, 0.6) !important',
      },
      '& .react-jinke-music-player-mobile-progress': {
        display: (props) => (props.isRadio ? 'none' : 'flex'),
      },
    },
  }),
  { name: 'NDAudioPlayer' },
)

export default useStyle

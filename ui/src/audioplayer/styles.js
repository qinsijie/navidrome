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
      '& .music-player-panel': {
        position: 'fixed !important',
        bottom: '0 !important',
        left: '0 !important',
        width: '100vw !important',
        height: '72px !important',
        zIndex: '1300 !important',
        boxShadow: '0 -2px 10px rgba(0, 0, 0, 0.15) !important',
        backgroundColor: `${theme.palette.background.paper} !important`,
        borderTop: `1px solid ${theme.palette.divider} !important`,
        color: `${theme.palette.text.primary} !important`,
        transition: 'none !important',
        transform: 'none !important',
      },
      '& .music-player-panel .panel-content': {
        height: '72px !important',
        display: 'flex !important',
        alignItems: 'center !important',
        justifyContent: 'space-between !important',
        padding: '0 24px !important',
        boxSizing: 'border-box !important',
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
      '& .music-player-panel .panel-content div.img-rotate': {
        // Customize desktop player when cover animation is disabled
        animationDuration: (props) => !props.enableCoverAnimation && '0s',
        borderRadius: (props) => !props.enableCoverAnimation && '0',
        // Fix cover display when image is not square
        backgroundSize: 'contain',
        backgroundPosition: 'center',
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
      '& .react-jinke-music-player-mobile-progress': {
        display: (props) => (props.isRadio ? 'none' : 'flex'),
      },
    },
  }),
  { name: 'NDAudioPlayer' },
)

export default useStyle

import React, {
  createElement,
  forwardRef,
  Fragment,
  useEffect,
  useState,
} from 'react'
import {
  AppBar as RAAppBar,
  MenuItemLink,
  useTranslate,
  usePermissions,
  getResources,
} from 'react-admin'
import { useHistory } from 'react-router-dom'
import {
  MdInfo,
  MdPerson,
  MdPhonelink,
  MdSupervisorAccount,
} from 'react-icons/md'
import { useSelector } from 'react-redux'
import {
  makeStyles,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Divider,
  IconButton,
  Snackbar,
  Tooltip,
  Typography,
} from '@material-ui/core'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import ViewListIcon from '@material-ui/icons/ViewList'
import OpenInBrowserIcon from '@material-ui/icons/OpenInBrowser'
import LanguageIcon from '@material-ui/icons/Language'
import FileCopyIcon from '@material-ui/icons/FileCopy'
import { Dialogs } from '../dialogs/Dialogs'
import { AboutDialog, QuickConnectDialog } from '../dialogs'
import PersonalMenu from './PersonalMenu'
import AudioSettingsMenu from './AudioSettingsMenu'
import ActivityPanel from './ActivityPanel'
import NowPlayingPanel from './NowPlayingPanel'
import UserMenu from './UserMenu'
import config from '../config'

const useStyles = makeStyles(
  (theme) => ({
    root: {
      color: theme.palette.text.secondary,
    },
    active: {
      color: theme.palette.text.primary,
    },
    icon: { minWidth: theme.spacing(5) },
    backButton: {
      marginRight: theme.spacing(0.5),
      color: 'inherit',
      '@media (max-width: 768px)': {
        display: 'none !important',
      },
    },
    mobileOnlyBtn: {
      color: 'inherit',
      '@media (min-width: 769px)': {
        display: 'none !important',
      },
    },
    title: {
      flex: 1,
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
    },
  }),
  {
    name: 'NDAppBar',
  },
)

export const NavBackButton = () => {
  const classes = useStyles()
  const history = useHistory()
  const [depth, setDepth] = useState(1)

  useEffect(() => {
    if (!history || typeof history.listen !== 'function') {
      return undefined
    }
    const unlisten = history.listen((_location, action) => {
      if (action === 'PUSH') {
        setDepth((d) => d + 1)
      } else if (action === 'POP') {
        setDepth((d) => Math.max(1, d - 1))
      }
    })
    return () => {
      if (typeof unlisten === 'function') {
        unlisten()
      }
    }
  }, [history])

  const canGoBack = depth > 1

  const handleBack = () => {
    if (!canGoBack) return
    if (history && typeof history.goBack === 'function') {
      history.goBack()
    } else if (window.history) {
      window.history.back()
    }
  }

  return (
    <Tooltip title="返回上一页 (Back)">
      <span className={classes.backButton}>
        <IconButton
          color="inherit"
          size="small"
          disabled={!canGoBack}
          onClick={handleBack}
          aria-label="back"
          data-testid="nav-back-button"
        >
          <ArrowBackIcon fontSize="small" />
        </IconButton>
      </span>
    </Tooltip>
  )
}

const DialogMenuItem = forwardRef(
  ({ onClick, label, icon, dialog, ...rest }, ref) => {
    const classes = useStyles(rest)
    const [open, setOpen] = React.useState(false)

    const handleClose = () => {
      onClick && onClick()
      setOpen(false)
    }
    return (
      <>
        <MenuItem
          ref={ref}
          onClick={() => setOpen(true)}
          className={classes.root}
        >
          <ListItemIcon className={classes.icon}>
            {createElement(icon, { title: label, size: 24 })}
          </ListItemIcon>
          {label}
        </MenuItem>
        {createElement(dialog, { onClose: handleClose, open })}
      </>
    )
  },
)

DialogMenuItem.displayName = 'DialogMenuItem'

const settingsResources = (resource) =>
  resource.name !== 'user' &&
  resource.hasList &&
  resource.options &&
  resource.options.subMenu === 'settings'

const copyTextCompat = async (text) => {
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

const OpenInBrowserButton = () => {
  const classes = useStyles()
  const [anchorEl, setAnchorEl] = useState(null)
  const [toastMsg, setToastMsg] = useState('')

  const pauseLocalBeforeSwitch = () => {
    window.dispatchEvent(new CustomEvent('nd:force-local-pause'))
  }

  const getTargetUrl = () => window.location.href

  const handleOpenSystem = () => {
    setAnchorEl(null)
    pauseLocalBeforeSwitch()
    const url = getTargetUrl()
    try {
      window.open(url, '_blank', 'noopener,noreferrer')
    } catch {
      // ignore
    }
    setToastMsg('已暂停本机播放，请在外部浏览器继续（可点右上角 × 关闭当前窗口）')
  }

  const handleOpenAndroidBrowser = (pkg, label) => {
    setAnchorEl(null)
    pauseLocalBeforeSwitch()
    const url = getTargetUrl()
    try {
      const parsed = new URL(url)
      const scheme = parsed.protocol.replace(':', '')
      const hostPath = `${parsed.host}${parsed.pathname}${parsed.search}${parsed.hash}`
      const intentUrl = `intent://${hostPath}#Intent;scheme=${scheme};package=${pkg};S.browser_fallback_url=${encodeURIComponent(url)};end`
      window.location.href = intentUrl
    } catch {
      window.open(url, '_blank', 'noopener,noreferrer')
    }
    setToastMsg(`已暂停本机播放，正在尝试唤起 ${label}（可点右上角 × 关闭当前窗口）`)
  }

  const handleCopyUrl = async () => {
    setAnchorEl(null)
    pauseLocalBeforeSwitch()
    const ok = await copyTextCompat(getTargetUrl())
    if (ok) {
      setToastMsg('已暂停本机播放并复制链接，请粘贴到外部浏览器打开')
    } else {
      setToastMsg('已暂停本机播放，请手动复制地址栏链接到浏览器打开')
    }
  }

  return (
    <>
      <Tooltip title="使用浏览器打开">
        <IconButton
          color="inherit"
          size="small"
          className={classes.mobileOnlyBtn}
          onClick={(e) => setAnchorEl(e.currentTarget)}
          aria-label="open-in-browser"
        >
          <OpenInBrowserIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={() => setAnchorEl(null)}
        getContentAnchorEl={null}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <MenuItem onClick={handleOpenSystem}>
          <ListItemIcon style={{ minWidth: 34 }}>
            <LanguageIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="系统默认浏览器打开" />
        </MenuItem>
        <MenuItem
          onClick={() =>
            handleOpenAndroidBrowser('com.android.chrome', 'Chrome')
          }
        >
          <ListItemIcon style={{ minWidth: 34 }}>
            <OpenInBrowserIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="Chrome 打开 (Android)" />
        </MenuItem>
        <MenuItem
          onClick={() =>
            handleOpenAndroidBrowser('com.microsoft.emmx', 'Edge')
          }
        >
          <ListItemIcon style={{ minWidth: 34 }}>
            <OpenInBrowserIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="Edge 打开 (Android)" />
        </MenuItem>
        <MenuItem
          onClick={() =>
            handleOpenAndroidBrowser('org.mozilla.firefox', 'Firefox')
          }
        >
          <ListItemIcon style={{ minWidth: 34 }}>
            <OpenInBrowserIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="Firefox 打开 (Android)" />
        </MenuItem>
        <Divider />
        <MenuItem onClick={handleCopyUrl}>
          <ListItemIcon style={{ minWidth: 34 }}>
            <FileCopyIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="复制当前链接去浏览器打开" />
        </MenuItem>
      </Menu>
      <Snackbar
        open={Boolean(toastMsg)}
        autoHideDuration={4500}
        onClose={() => setToastMsg('')}
        message={toastMsg}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      />
    </>
  )
}

const CustomUserMenu = ({ onClick, ...rest }) => {
  const translate = useTranslate()
  const resources = useSelector(getResources)
  const classes = useStyles(rest)
  const { permissions } = usePermissions()

  const resourceDefinition = (resourceName) =>
    resources.find((r) => r?.name === resourceName)

  const renderUserMenuItemLink = () => {
    const userResource = resourceDefinition('user')
    if (!userResource) {
      return null
    }
    if (permissions !== 'admin') {
      if (!config.enableUserEditing) {
        return null
      }
      userResource.icon = MdPerson
    } else {
      userResource.icon = MdSupervisorAccount
    }
    return renderSettingsMenuItemLink(
      userResource,
      permissions !== 'admin' ? localStorage.getItem('userId') : null,
    )
  }

  const renderSettingsMenuItemLink = (resource, id) => {
    const label = resource.options.label
      ? translate(resource.options.label)
      : translate(`resources.${resource.name}.name`, {
          smart_count: id ? 1 : 2,
        })
    const link = id ? `/${resource.name}/${id}` : `/${resource.name}`
    return (
      <MenuItemLink
        className={classes.root}
        activeClassName={classes.active}
        key={resource.name}
        to={link}
        primaryText={label}
        leftIcon={
          (resource.icon && createElement(resource.icon, { size: 24 })) || (
            <ViewListIcon />
          )
        }
        onClick={onClick}
        sidebarIsOpen={true}
      />
    )
  }

  return (
    <>
      {config.devActivityPanel &&
        permissions === 'admin' &&
        config.enableNowPlaying && <NowPlayingPanel />}
      {config.devActivityPanel && permissions === 'admin' && <ActivityPanel />}
      <OpenInBrowserButton />
      <UserMenu {...rest}>
        <PersonalMenu sidebarIsOpen={true} onClick={onClick} />
        <AudioSettingsMenu sidebarIsOpen={true} onClick={onClick} />
        {config.enableQuickConnect && (
          <DialogMenuItem
            label={translate('menu.quickConnect.name')}
            icon={MdPhonelink}
            dialog={QuickConnectDialog}
          />
        )}
        <Divider />
        {renderUserMenuItemLink()}
        {resources
          .filter(settingsResources)
          .map((r) => renderSettingsMenuItemLink(r))}
        <Divider />
        <DialogMenuItem
          label={translate('menu.about')}
          icon={MdInfo}
          dialog={AboutDialog}
        />
      </UserMenu>
      <Dialogs />
    </>
  )
}

const AppBar = (props) => {
  const classes = useStyles(props)
  return (
    <RAAppBar
      {...props}
      container={Fragment}
      userMenu={<CustomUserMenu />}
    >
      <NavBackButton />
      <Typography
        variant="h6"
        color="inherit"
        className={classes.title}
        id="react-admin-title"
      />
    </RAAppBar>
  )
}

export default AppBar

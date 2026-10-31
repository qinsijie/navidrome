import React, { forwardRef } from 'react'
import { MenuItemLink } from 'react-admin'
import { MdSpeaker } from 'react-icons/md'
import { makeStyles } from '@material-ui/core'

const useStyles = makeStyles((theme) => ({
  menuItem: {
    color: theme.palette.text.secondary,
  },
}))

const AudioSettingsMenu = forwardRef(({ onClick, sidebarIsOpen, dense }, ref) => {
  const classes = useStyles()
  return (
    <MenuItemLink
      ref={ref}
      to="/settings/audio"
      primaryText="音频输出与串流"
      leftIcon={<MdSpeaker size={24} />}
      onClick={onClick}
      className={classes.menuItem}
      sidebarIsOpen={sidebarIsOpen}
      dense={dense}
    />
  )
})

AudioSettingsMenu.displayName = 'AudioSettingsMenu'

export default AudioSettingsMenu

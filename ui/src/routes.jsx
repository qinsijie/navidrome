import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudioSettings from './personal/AudioSettings'
import {
  OnlineMusicSearch,
  OnlineMusicPlaylists,
  OnlineLeaderboards,
  SourceSettings,
} from './onlinemusic'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route
    exact
    path="/settings/audio"
    render={() => <AudioSettings />}
    key={'settings-audio'}
  />,
  <Route
    exact
    path="/online-music/search"
    render={() => <OnlineMusicSearch />}
    key={'online-music-search'}
  />,
  <Route
    exact
    path="/online-music/playlists"
    render={() => <OnlineMusicPlaylists />}
    key={'online-music-playlists'}
  />,
  <Route
    exact
    path="/online-music/leaderboards"
    render={() => <OnlineLeaderboards />}
    key={'online-music-leaderboards'}
  />,
  <Route
    exact
    path="/online-music/settings"
    render={() => <SourceSettings />}
    key={'online-music-settings'}
  />,
]

export default routes

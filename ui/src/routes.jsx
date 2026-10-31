import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudioSettings from './personal/AudioSettings'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route
    exact
    path="/settings/audio"
    render={() => <AudioSettings />}
    key={'settings-audio'}
  />,
]

export default routes

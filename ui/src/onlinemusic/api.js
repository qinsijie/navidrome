import { httpClient } from '../dataProvider'

export const searchOnline = async (source, query, page = 1, limit = 30) => {
  const url = `/api/musicengine/search?source=${encodeURIComponent(source)}&query=${encodeURIComponent(query)}&page=${page}&limit=${limit}`
  const response = await httpClient(url)
  return response.json
}

export const ensureShadowTrack = async (track) => {
  const response = await httpClient('/api/musicengine/shadow', {
    method: 'POST',
    body: JSON.stringify(track),
  })
  return response.json
}

export const getSonglists = async (source) => {
  const url = `/api/musicengine/songlists?source=${encodeURIComponent(source)}`
  const response = await httpClient(url)
  return response.json
}

export const getSonglistDetail = async (source, id) => {
  const url = `/api/musicengine/songlist?source=${encodeURIComponent(source)}&id=${encodeURIComponent(id)}`
  const response = await httpClient(url)
  return response.json
}

export const getLeaderboards = async (source) => {
  const url = `/api/musicengine/leaderboards?source=${encodeURIComponent(source)}`
  const response = await httpClient(url)
  return response.json
}

export const getLeaderboardDetail = async (source, id) => {
  const url = `/api/musicengine/leaderboard?source=${encodeURIComponent(source)}&id=${encodeURIComponent(id)}`
  const response = await httpClient(url)
  return response.json
}

export const getSources = async () => {
  const response = await httpClient('/api/musicengine/sources')
  return response.json
}

export const uploadSource = async (name, description, script) => {
  const response = await httpClient('/api/musicengine/sources/upload', {
    method: 'POST',
    body: JSON.stringify({ name, description, script }),
  })
  return response.json
}

import { httpClient } from '../dataProvider'

export const searchOnline = async (source, query, page = 1, limit = 30) => {
  const url = `/api/musicengine/search?source=${encodeURIComponent(source)}&query=${encodeURIComponent(query)}&page=${page}&limit=${limit}`
  const response = await httpClient(url)
  return response.json
}

export const getSearchTips = async (query) => {
  const url = `/api/musicengine/search/tips?query=${encodeURIComponent(query)}`
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

export const uploadSource = async (name, description, script, subscriptionUrl = '') => {
  const response = await httpClient('/api/musicengine/sources/upload', {
    method: 'POST',
    body: JSON.stringify({ name, description, script, subscriptionUrl }),
  })
  return response.json
}

export const importSourceFromUrl = async (url) => {
  const response = await httpClient('/api/musicengine/sources/import-url', {
    method: 'POST',
    body: JSON.stringify({ url }),
  })
  return response.json
}

export const activateSource = async (id) => {
  const response = await httpClient(`/api/musicengine/sources/${encodeURIComponent(id)}/activate`, {
    method: 'POST',
  })
  return response.json
}

export const deleteSource = async (id) => {
  const response = await httpClient(`/api/musicengine/sources/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  return response.json
}

export const checkSourceUpdate = async (id) => {
  const response = await httpClient(`/api/musicengine/sources/${encodeURIComponent(id)}/check-update`, {
    method: 'POST',
  })
  return response.json
}

export const pingSources = async () => {
  const response = await httpClient('/api/musicengine/sources/ping', {
    method: 'POST',
  })
  return response.json
}

export const getOnlineCacheStats = async () => {
  const response = await httpClient('/api/musicengine/cache')
  return response.json
}

export const setOnlineCacheConfig = async (maxMB) => {
  const response = await httpClient('/api/musicengine/cache/config', {
    method: 'POST',
    body: JSON.stringify({ maxMB }),
  })
  return response.json
}

export const clearOnlineCache = async () => {
  const response = await httpClient('/api/musicengine/cache', {
    method: 'DELETE',
  })
  return response.json
}

export const downloadSourceScript = async (id) => {
  const token = localStorage.getItem('token')
  const headers = {}
  if (token) {
    headers['x-nd-authorization'] = `Bearer ${token}`
  }
  const response = await fetch(
    `/api/musicengine/sources/${encodeURIComponent(id)}/download`,
    { headers },
  )
  if (!response.ok) {
    throw new Error(`下载失败 (${response.status})`)
  }
  const blob = await response.blob()
  const disp = response.headers.get('content-disposition') || ''
  let fileName = `${id}.js`
  const utf8Match = disp.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match && utf8Match[1]) {
    try {
      fileName = decodeURIComponent(utf8Match[1])
    } catch {
      fileName = utf8Match[1]
    }
  } else {
    const plainMatch = disp.match(/filename="?([^"]+)"?/i)
    if (plainMatch && plainMatch[1]) {
      fileName = plainMatch[1]
    }
  }
  return { blob, fileName }
}

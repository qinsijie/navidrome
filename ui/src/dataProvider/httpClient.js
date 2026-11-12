import { fetchUtils } from 'react-admin'
import { v4 as uuidv4 } from 'uuid'
import { baseUrl } from '../utils'
import config from '../config'
import { jwtDecode } from 'jwt-decode'

const customAuthorizationHeader = 'X-ND-Authorization'
export const clientUniqueIdHeader = 'X-ND-Client-Unique-Id'

export const getClientUniqueId = () => {
  let id = localStorage.getItem('nd_client_unique_id')
  if (!id) {
    id = 'cli_' + uuidv4().replace(/-/g, '').slice(0, 12)
    localStorage.setItem('nd_client_unique_id', id)
  }
  return id
}

export const getClientName = () => {
  let name = localStorage.getItem('nd_client_name')
  if (!name) {
    const id = getClientUniqueId()
    const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
    let platform = '终端'
    if (/iPhone/i.test(ua)) platform = 'iPhone'
    else if (/iPad/i.test(ua)) platform = 'iPad'
    else if (/Android/i.test(ua)) platform = '安卓手机'
    else if (/Macintosh|Mac OS X/i.test(ua)) platform = 'Mac'
    else if (/Windows/i.test(ua)) platform = '电脑'
    name = `${platform}-${id.slice(-4)}`
    localStorage.setItem('nd_client_name', name)
  }
  return name
}

export const setClientName = (name) => {
  if (name && name.trim()) {
    localStorage.setItem('nd_client_name', name.trim())
    window.dispatchEvent(new CustomEvent('nd:client-name-updated', { detail: { name: name.trim() } }))
  }
}

export const clientUniqueId = getClientUniqueId()

const httpClient = (url, options = {}) => {
  url = baseUrl(url)
  if (!options.headers) {
    options.headers = new Headers({ Accept: 'application/json' })
  }
  options.headers.set(clientUniqueIdHeader, clientUniqueId)
  const token = localStorage.getItem('token')
  if (token) {
    options.headers.set(customAuthorizationHeader, `Bearer ${token}`)
  }
  return fetchUtils.fetchJson(url, options).then((response) => {
    const token = response.headers.get(customAuthorizationHeader)
    if (token) {
      const decoded = jwtDecode(token)
      localStorage.setItem('token', token)
      localStorage.setItem('userId', decoded.uid)
      // Avoid going to create admin dialog after logout/login without a refresh
      config.firstTime = false
    }
    return response
  })
}

export default httpClient

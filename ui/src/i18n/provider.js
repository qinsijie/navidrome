import polyglotI18nProvider from 'ra-i18n-polyglot'
import deepmerge from 'deepmerge'
import dataProvider from '../dataProvider'
import en from './en.json'
import zhHans from './zh-Hans.json'
import config from '../config'

const removeEmpty = (obj) => {
  for (let k in obj) {
    if (
      Object.prototype.hasOwnProperty.call(obj, k) &&
      typeof obj[k] === 'object'
    ) {
      removeEmpty(obj[k])
    } else {
      if (!obj[k]) {
        delete obj[k]
      }
    }
  }
}

const prepareLanguage = (lang) => {
  removeEmpty(lang)
  // Aliases below go on the merged copy: mutating `en` would corrupt the completion baseline
  const merged = deepmerge(en, lang)
  // Make "albumSong" and "playlistTrack" resource use the same translations as "song"
  merged.resources.albumSong = merged.resources.song
  merged.resources.playlistTrack = merged.resources.song
  // ra.boolean.null should always be empty
  merged.ra.boolean.null = ''
  return merged
}

const bundledLanguages = {
  en: prepareLanguage(en),
  'zh-Hans': prepareLanguage(zhHans),
}

// Always default to Chinese for fnOS unless user explicitly specified another locale
const defaultLocale = function () {
  const locale = localStorage.getItem('locale')
  if (locale && (bundledLanguages[locale] || localStorage.getItem('translation'))) {
    return locale
  }
  if (config.defaultLanguage) {
    return config.defaultLanguage
  }
  return 'zh-Hans'
}

export function retrieveTranslation(locale) {
  if (bundledLanguages[locale]) {
    return Promise.resolve(bundledLanguages[locale])
  }
  return dataProvider.getOne('translation', { id: locale }).then((res) => {
    localStorage.setItem('translation', JSON.stringify(res.data))
    return prepareLanguage(JSON.parse(res.data.data))
  })
}

export default polyglotI18nProvider((locale) => {
  // Check bundled translations first (English and Simplified Chinese)
  if (bundledLanguages[locale]) {
    return bundledLanguages[locale]
  }
  // If the requested locale is in already loaded, return it
  const current = JSON.parse(localStorage.getItem('translation'))
  if (current && current.id === locale) {
    return prepareLanguage(JSON.parse(current.data))
  }
  // If not, get it from the server, and store it in localStorage
  return retrieveTranslation(locale)
}, defaultLocale())

import { describe, it, expect } from 'vitest'
import {
  isValidTrackId,
  resolveTrackId,
  buildRemoteOutputTrack,
} from './trackIdUtils'

describe('Player track ID resolution and guards', () => {
  it('validates track IDs strictly', () => {
    expect(isValidTrackId('abc123')).toBe(true)
    expect(isValidTrackId('online:kw:12345')).toBe(true)
    expect(isValidTrackId('')).toBe(false)
    expect(isValidTrackId('   ')).toBe(false)
    expect(isValidTrackId(undefined)).toBe(false)
    expect(isValidTrackId(null)).toBe(false)
    expect(isValidTrackId('undefined')).toBe(false)
    expect(isValidTrackId('null')).toBe(false)
  })

  it('resolves ID from id, ID, mediaFileId, trackId, or nested song', () => {
    expect(resolveTrackId({ id: 't1' })).toBe('t1')
    expect(resolveTrackId({ ID: 't2' })).toBe('t2')
    expect(resolveTrackId({ mediaFileId: 't3' })).toBe('t3')
    expect(resolveTrackId({ trackId: 't4' })).toBe('t4')
    expect(resolveTrackId({ id: 'undefined', trackId: 't5' })).toBe('t5')
    expect(resolveTrackId({ song: { id: 't6' } })).toBe('t6')
    expect(resolveTrackId({ id: 'undefined', trackId: null })).toBe('')
    expect(resolveTrackId(null)).toBe('')
  })

  it('builds remote output track and blocks undefined.wav', () => {
    expect(
      buildRemoteOutputTrack({
        id: 'track-99',
        song: { title: 'Song A', artist: 'Artist B', album: 'Album C', duration: 210 },
      }),
    ).toEqual({
      trackId: 'track-99',
      title: 'Song A',
      artist: 'Artist B',
      album: 'Album C',
      duration: 210,
      streamUrl: '/api/output/stream/track-99',
    })

    expect(buildRemoteOutputTrack({ id: 'undefined', name: 'Bad' })).toBeNull()
    expect(buildRemoteOutputTrack({ trackId: '', name: 'Empty' })).toBeNull()
    expect(buildRemoteOutputTrack({})).toBeNull()
  })
})

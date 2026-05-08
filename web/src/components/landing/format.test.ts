import { describe, expect, test } from 'bun:test'
import { plural, formatFragTime } from './format'

describe('plural', () => {
  test('returns singular when n=1', () => {
    expect(plural(1, 'player', 'players')).toBe('player')
  })
  test('returns plural when n=0', () => {
    expect(plural(0, 'player', 'players')).toBe('players')
  })
  test('returns plural when n=2', () => {
    expect(plural(2, 'arena', 'arenas')).toBe('arenas')
  })
})

describe('formatFragTime', () => {
  test('returns empty string for less than 60s', () => {
    expect(formatFragTime(0)).toBe('')
    expect(formatFragTime(59)).toBe('')
  })
  test('formats minutes-only when under an hour', () => {
    expect(formatFragTime(60)).toBe('1m of fragging')
    expect(formatFragTime(42 * 60)).toBe('42m of fragging')
  })
  test('formats hours and zero-padded minutes when over an hour', () => {
    expect(formatFragTime(60 * 60)).toBe('1h 00m of fragging')
    expect(formatFragTime(4 * 3600 + 18 * 60)).toBe('4h 18m of fragging')
    expect(formatFragTime(4 * 3600 + 3 * 60)).toBe('4h 03m of fragging')
  })
})

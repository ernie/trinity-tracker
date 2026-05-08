import { describe, expect, test } from 'bun:test'
import { formatExitReason, classifyScores } from './format'

describe('formatExitReason', () => {
  test('maps known reasons to prose', () => {
    expect(formatExitReason('score_limit')).toBe('Score limit reached')
    expect(formatExitReason('time_limit')).toBe('Time limit')
    expect(formatExitReason('overtime')).toBe('Decided in OT')
    expect(formatExitReason('forfeit')).toBe('Forfeit')
    expect(formatExitReason('intermission')).toBe('Match ended')
  })
  test('falls back to titlecased input for unknown reasons', () => {
    expect(formatExitReason('weird_thing')).toBe('Weird thing')
  })
  test('handles empty/undefined', () => {
    expect(formatExitReason('')).toBe('')
    expect(formatExitReason(undefined)).toBe('')
  })
})

describe('classifyScores', () => {
  test('detects no-contest when both scores are <= 0', () => {
    expect(classifyScores(0, 0)).toBe('no_contest')
    expect(classifyScores(-1, 0)).toBe('no_contest')
  })
  test('detects tie when scores are equal and positive', () => {
    expect(classifyScores(22, 22)).toBe('tie')
    expect(classifyScores(100, 100)).toBe('tie')
  })
  test('detects winner', () => {
    expect(classifyScores(25, 22)).toBe('left')
    expect(classifyScores(22, 25)).toBe('right')
  })
})

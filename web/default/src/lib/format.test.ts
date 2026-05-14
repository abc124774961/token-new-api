import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { formatUseTime } from './format'

describe('formatUseTime', () => {
  test('returns dash for non-finite values', () => {
    assert.equal(formatUseTime(Number.NaN), '-')
    assert.equal(formatUseTime(Number.POSITIVE_INFINITY), '-')
    assert.equal(formatUseTime(-1), '-')
  })

  test('formats seconds and minutes', () => {
    assert.equal(formatUseTime(12.3), '12.3s')
    assert.equal(formatUseTime(125), '2m 5s')
  })
})

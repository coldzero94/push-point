import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

import { coverPattern, hashDomain } from './covers'

// The SAME file iOS reads (ios/PushPointTests/CoverPatternTests.swift). Before
// 2026-08-03 the expected numbers lived only in the Swift test, under a comment
// saying they had to match the web — and nothing on the web side ever checked.
// They did not match: `>>` coerces to int32, so every domain whose FNV-1a hash
// has the high bit set produced different geometry here than on iOS, and one of
// them (`news.ycombinator.com`, step -4) threw out of canvas `arc` and took the
// whole list screen down.
const fixture = JSON.parse(
  readFileSync(new URL('../../../testdata/cover-cases.json', import.meta.url), 'utf8'),
) as {
  cases: {
    domain: string
    seed: number
    kind: string
    rotate: number
    step: number
    variant: number
  }[]
}

describe('coverPattern', () => {
  it.each(fixture.cases)('matches the shared fixture: $domain', (c) => {
    expect(hashDomain(c.domain)).toBe(c.seed)
    const got = coverPattern(c.domain)
    expect(got.kind).toBe(c.kind)
    expect(got.rotate).toBe(c.rotate)
    expect(got.step).toBe(c.step)
    expect(got.variant).toBe(c.variant)
  })

  // The fixture is only worth having if it exercises the half of the hash space
  // that the old hand-written golden missed.
  it('covers hashes with the high bit set', () => {
    expect(fixture.cases.filter((c) => c.seed >= 2 ** 31).length).toBeGreaterThan(0)
  })

  // A negative step reaches canvas `arc` as a negative radius, which throws
  // rather than drawing badly — one bad domain in a list kills the screen.
  it('never produces geometry that canvas rejects', () => {
    const domains = [...fixture.cases.map((c) => c.domain)]
    for (let i = 0; i < 5000; i++) domains.push(`d${i}.example.com`, `${i}.co.kr`, `한글${i}.kr`)
    for (const d of domains) {
      const p = coverPattern(d)
      expect(p.step, d).toBeGreaterThan(0)
      expect(p.rotate, d).toBeGreaterThanOrEqual(-2)
      expect(p.rotate, d).toBeLessThanOrEqual(2)
      expect(p.variant, d).toBeGreaterThanOrEqual(0)
      expect(p.variant, d).toBeLessThanOrEqual(4)
    }
  })
})

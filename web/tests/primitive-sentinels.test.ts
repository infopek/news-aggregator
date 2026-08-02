import { readFileSync } from 'node:fs'
import { globSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('credential lifecycle source guard', () => {
  it('contains no persistence, logging, HTML trust, or snapshot mechanism', () => {
    const files = globSync('src/{components,testing}/**/*.{vue,ts}', { cwd: process.cwd() })
    const source = files.map((file) => readFileSync(file, 'utf8')).join('\n')
    expect(source).not.toMatch(/localStorage\.(setItem|getItem)|sessionStorage\.(setItem|getItem)|console\.(log|debug|info)|v-html|innerHTML|toMatchSnapshot/)
  })
})

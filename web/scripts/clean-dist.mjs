import { mkdir, readdir, rm, writeFile } from 'node:fs/promises'
import { fileURLToPath, URL } from 'node:url'

const distUrl = new URL('../../internal/webassets/dist/', import.meta.url)
const dist = fileURLToPath(distUrl)

await mkdir(dist, { recursive: true })

for (const entry of await readdir(dist)) {
  if (entry !== '.gitkeep') {
    await rm(new URL(entry, distUrl), { force: true, recursive: true })
  }
}

await writeFile(new URL('.gitkeep', distUrl), 'Frontend builds preserve this placeholder.\n')

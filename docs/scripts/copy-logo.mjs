// The browser tab wants a file rather than a component, so the wordmark is
// copied in for that alone; everywhere else the site draws it inline, from
// app/_components/wordmark.jsx.
import { cp } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const SITE = path.join(path.dirname(fileURLToPath(import.meta.url)), '..')

await cp(
  path.join(SITE, '../misc/jmapc.svg'),
  path.join(SITE, 'public/logo.svg')
)

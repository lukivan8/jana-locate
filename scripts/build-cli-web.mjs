// Adopt the independently built frontend as an offline CLI release artifact.
import { execFileSync } from 'node:child_process'
import { cpSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
const root = dirname(dirname(fileURLToPath(import.meta.url)))
const frontend = join(root, 'frontend')
function files(path) { return readdirSync(path, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? files(join(path, entry.name)) : [join(path, entry.name)]) }
const source = [...files(join(frontend, 'src')), ...['package.json', 'package-lock.json', 'vite.config.ts', 'tsconfig.json'].map(name => join(frontend, name))].sort()
const manifest = Object.fromEntries(source.map(path => [relative(frontend, path), createHash('sha256').update(readFileSync(path)).digest('hex')]))
const target = join(root, 'cli/web/static')
if (process.argv.includes('--check')) {
  const saved = JSON.parse(readFileSync(join(root, 'cli/web/frontend-source.json'), 'utf8'))
  if (JSON.stringify(manifest) !== JSON.stringify(saved)) throw new Error('CLI frontend snapshot is stale. Run node scripts/build-cli-web.mjs.')
  console.log('CLI embeds the current shared frontend source.')
} else {
  execFileSync('npm', ['run', 'build'], { cwd: frontend, stdio: 'inherit' })
  rmSync(target, { recursive: true, force: true })
  mkdirSync(target, { recursive: true })
  cpSync(join(frontend, 'dist/client'), target, { recursive: true })
  // Runtime config comes from the CLI server, never from the adopted frontend file.
  rmSync(join(target, 'config.js'))
  writeFileSync(join(root, 'cli/web/frontend-source.json'), JSON.stringify(manifest, null, 2) + '\n')
}

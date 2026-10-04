// 资源缺失核对脚本：扫描 frontend/src 中对 /images/ 资产的引用，与 public/images 实际文件比对，报告缺失。
// 用法：node scripts/check-assets.mjs
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, dirname, normalize, extname } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRCDIR = join(ROOT, 'src')
const PUBDIR = join(ROOT, 'public', 'images')

// 1) 建立 public/images 下全部相对路径集合（正斜杠）。
function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p).forEach((f) => out.push(f))
    else out.push(p)
  }
  return out
}
const existing = new Set(walk(PUBDIR).map((p) => normalize(p).replace(/\\/g, '/').replace(/^[^:]+:\/|^\/+/, '')))
const relToImgs = (f) => normalize(f).replace(/\\/g, '/').replace(/.*\/public\/images\//, '')

// 2) 扫描 src 下所有文件文本。
function scanSrc() {
  const files = []
  const stack = [SRCDIR]
  while (stack.length) {
    const dir = stack.pop()
    for (const name of readdirSync(dir)) {
      const p = join(dir, name)
      const st = statSync(p)
      if (st.isDirectory()) stack.push(p)
      else if (name.endsWith('.ts') || name.endsWith('.vue') || name.endsWith('.tsx')) files.push(p)
    }
  }
  return files
}

const refs = []
for (const f of scanSrc()) {
  const text = readFileSync(f, 'utf8')
  // img('rel')
  for (const m of text.matchAll(/'\/images\/+([^'"]+)'/g)) refs.push([f, m[1]])
  for (const m of text.matchAll(/"\/images\/+([^"]+)"/g)) refs.push([f, m[1]])
  // img(`rel`) 字面模板：仅当不含插值才可核对
  for (const m of text.matchAll(/`\/images\/+([^`]+)`/g)) {
    if (!/\$\{/.test(m[1])) refs.push([f, m[1]])
  }
}

// throwaway: 去掉行尾非路径字符
const clean = (s) => s.split(/\s|['"`\)]/)[0]

const missing = []
const seen = new Set()
for (const [file, ref] of refs) {
  const r = clean(ref).replace(/^\/+/, '')
  const key = file + r
  if (!r || seen.has(key)) continue
  seen.add(key)
  const exists = existing.has(r)
  if (!exists) missing.push({ file, ref: r })
}

console.log(`已核对引用 ${seen.size} 条；缺失 ${missing.length} 条`)
if (missing.length) {
  for (const m of missing) console.log(`  MISSING  ${m.ref}  (${m.file})`)
  process.exit(1)
}
console.log('OK: 无缺失资源')
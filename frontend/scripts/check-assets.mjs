// 资源核对脚本（整理版）：四段核对
//   1) 缺失核对：src 中【字面量】/images/ 引用 与 public/images 实际文件比对
//   2) 动态规则核对：从 src 的 img(`...`) 模板提取命名规则，统计各规则命中的资源数
//   3) 未使用清单：public/images 中未被任何字面量或动态规则命中的文件（候选冗余）
//   4) 交叉核对：tools/asset-index.json 的 cross.referencedNotInSrc（AS 已引用、重写版未接入的资源族）
// 用法：node scripts/check-assets.mjs   （仅当【字面量引用缺失】时退出码 1）
import { readdirSync, readFileSync, statSync, existsSync } from 'node:fs'
import { join, dirname, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRCDIR = join(ROOT, 'src')
const PUBDIR = join(ROOT, 'public', 'images')

// ── 1) public/images 全部文件（正斜杠相对路径） ────────────────────────────
function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...walk(p))
    else out.push(p)
  }
  return out
}
const relOf = (f) => normalize(f).replace(/\\/g, '/').replace(/.*\/public\/images\//, '')
const files = walk(PUBDIR).map(relOf)
const existing = new Set(files)

// ── 2) 扫描 src 文本 ─────────────────────────────────────────────────────
function scanSrc() {
  const out = []
  const stack = [SRCDIR]
  while (stack.length) {
    const dir = stack.pop()
    for (const name of readdirSync(dir)) {
      const p = join(dir, name)
      const st = statSync(p)
      if (st.isDirectory()) stack.push(p)
      else if (/\.(ts|vue|tsx|css)$/.test(name)) out.push(p)
    }
  }
  return out
}
const srcFiles = scanSrc()

// 2a) 字面量引用：'...'、"..."、`...`（无插值）与 CSS url('/images/...')
const literals = []
for (const f of srcFiles) {
  const text = readFileSync(f, 'utf8')
  for (const m of text.matchAll(/['"`]\/images\/+([^'"`]+)['"`]/g)) {
    if (!m[1].includes('${')) literals.push([f, m[1]])
  }
}
const clean = (s) => s.split(/\s|['"`)]/)[0].replace(/^\/+/, '')

// 2b) 动态规则：img(`前缀${...}后缀`) → 正则
const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
const patterns = []
for (const f of srcFiles) {
  if (!/\.(ts|vue|tsx)$/.test(f)) continue
  const text = readFileSync(f, 'utf8')
  for (const m of text.matchAll(/img\(\s*`([^`]+)`\s*\)/g)) {
    const tpl = m[1]
    if (!tpl.includes('${')) continue
    // 逐段转正则：字面量转义，${...} → 任意非 / 字符
    const body = tpl
      .split(/\$\{[^}]*\}/)
      .map(escapeRe)
      .join('[^/]*')
    patterns.push({ file: f, tpl, re: new RegExp(`^${body}$`) })
  }
}

// ── 3) 核对 ──────────────────────────────────────────────────────────────
const missing = []
const seen = new Set()
const literalMatched = new Set()
for (const [file, raw] of literals) {
  const r = clean(raw)
  const key = file + r
  if (!r || seen.has(key)) continue
  seen.add(key)
  if (existing.has(r)) literalMatched.add(r)
  else missing.push({ file, ref: r })
}

const patternHits = new Map() // tpl → Set(files)
const patternMatched = new Set()
for (const p of patterns) {
  const hits = new Set()
  for (const f of files) {
    if (p.re.test(f)) {
      hits.add(f)
      patternMatched.add(f)
    }
  }
  if (!patternHits.has(p.tpl)) patternHits.set(p.tpl, { files: new Set(), where: new Set() })
  const rec = patternHits.get(p.tpl)
  hits.forEach((h) => rec.files.add(h))
  rec.where.add(p.file.replace(SRCDIR, 'src').replace(/\\/g, '/'))
}

const used = new Set([...literalMatched, ...patternMatched])
const unused = files.filter((f) => !used.has(f))

// ── 4) 报告 ──────────────────────────────────────────────────────────────
console.log('=== 1) 字面量引用核对 ===')
console.log(`核对 ${seen.size} 条；缺失 ${missing.length} 条`)
for (const m of missing) console.log(`  MISSING  ${m.ref}  (${m.file.replace(SRCDIR, 'src')})`)

console.log('\n=== 2) 动态规则（由 img(`...`) 模板提取） ===')
const sorted = [...patternHits.entries()].sort((a, b) => b[1].files.size - a[1].files.size)
for (const [tpl, rec] of sorted) {
  console.log(`  ${tpl.padEnd(34)} 命中 ${String(rec.files.size).padStart(5)} 个   ← ${[...rec.where].join(', ')}`)
}

console.log('\n=== 3) 未使用资源（未被任何字面量或动态规则命中） ===')
const byDir = new Map()
for (const f of unused) {
  const dir = f.includes('/') ? f.split('/')[0] + '/' : '(根目录)'
  byDir.set(dir, (byDir.get(dir) ?? 0) + 1)
}
const total = files.length
console.log(`资源总数 ${total}；被引用 ${used.size}；未使用 ${unused.length}（${((unused.length / total) * 100).toFixed(1)}%）`)
for (const [dir, n] of [...byDir.entries()].sort((a, b) => b[1] - a[1])) {
  console.log(`  ${dir.padEnd(18)} ${String(n).padStart(5)}`)
}
console.log('\n未使用明细（前 120 个，完整清单可用 --list 输出）：')
const limit = process.argv.includes('--list') ? unused.length : 120
for (const f of unused.slice(0, limit)) console.log(`  ${f}`)
if (unused.length > limit) console.log(`  ... 其余 ${unused.length - limit} 个（--list 查看全部）`)

// ── 5) 第 4 段：tools/asset-index.json 交叉核对 ─────────────────────────────
// AS 已引用（embed/filename/string/bare）、但重写版 src 尚未接入的资源族。
// frontCount>0 = 前端 public/images 已有文件，可直接接入；frontCount=0 = ⚠ 前端无文件。
console.log('\n=== 4) AS 已引用、重写版尚未接入的资源族（源：../tools/asset-index.json → cross.referencedNotInSrc） ===')
const indexPath = join(ROOT, '..', 'tools', 'asset-index.json')
if (!existsSync(indexPath)) {
  console.log('  未找到 ../tools/asset-index.json，跳过本段。')
} else {
  let cross = null
  try {
    cross = JSON.parse(readFileSync(indexPath, 'utf8'))?.cross?.referencedNotInSrc
  } catch (e) {
    console.log(`  读取失败：${e.message}`)
  }
  if (!Array.isArray(cross) || cross.length === 0) {
    console.log('  cross.referencedNotInSrc 为空或缺失。')
  } else {
    const rows = cross
      .map((it) => ({
        family: it.family,
        refCount: Number(it.refCount) || 0,
        frontCount: Number(it.frontCount) || 0,
      }))
      .sort((a, b) => b.refCount - a.refCount)
    const refTotal = rows.reduce((s, r) => s + r.refCount, 0)
    const hasFront = rows.filter((r) => r.frontCount > 0)
    const noFront = rows.filter((r) => r.frontCount <= 0)
    console.log(
      `  族数 ${rows.length}；AS 引用合计 ${refTotal} 次；` +
        `前端已有文件（frontCount>0，可直接接入）${hasFront.length} 族；⚠ 前端无文件 ${noFront.length} 族`,
    )
    console.log('  按 AS 引用次数降序（前 40）：')
    for (const r of rows.slice(0, 40)) {
      const tag = r.frontCount > 0 ? `\u2714 可直接接入 frontCount=${r.frontCount}` : '⚠ 前端无文件'
      console.log(`    refCount=${String(r.refCount).padStart(4)}  ${tag.padEnd(26)}  ${r.family}`)
    }
    if (rows.length > 40) console.log(`    ... 其余 ${rows.length - 40} 族（共 ${rows.length}）`)
  }
}

if (missing.length) process.exit(1)

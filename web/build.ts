import { rm, cp, readFile, writeFile } from 'node:fs/promises'
import { relative } from 'node:path'

const dist = './dist'

await rm(dist, { recursive: true, force: true })

const result = await Bun.build({
  entrypoints: ['./src/main.tsx'],
  outdir: dist,
  naming: 'assets/[name]-[hash].[ext]',
  minify: true,
  target: 'browser',
  external: ['/engine/*', '/assets/*', '/configs/*'],
  define: { 'process.env.NODE_ENV': '"production"' },
})

if (!result.success) {
  for (const log of result.logs) console.error(log)
  process.exit(1)
}

await cp('./public', dist, { recursive: true })

const href = (p: string) => '/' + relative(dist, p)
const js = result.outputs.find((o) => o.kind === 'entry-point')!
const css = result.outputs.find((o) => o.path.endsWith('.css'))

let html = await readFile('./index.html', 'utf8')
if (css) {
  html = html.replace(
    '</head>',
    `    <link rel="stylesheet" href="${href(css.path)}">\n  </head>`,
  )
}
html = html.replace(
  '<script type="module" src="/src/main.tsx"></script>',
  `<script type="module" src="${href(js.path)}"></script>`,
)
await writeFile(`${dist}/index.html`, html)

for (const o of result.outputs) {
  console.log(`  ${relative(process.cwd(), o.path)}  ${(o.size / 1024).toFixed(2)} kB`)
}

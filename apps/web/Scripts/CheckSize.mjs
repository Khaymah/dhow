import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, posix } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

import { evaluate, KB, overrideMatches, parseOverrides, withOverrides } from './SizeBudget.mjs';

const loadConfig = () => {
  const base = JSON.parse(readFileSync(fileURLToPath(new URL('./SizeBudgets.json', import.meta.url)), 'utf8'));
  try {
    return withOverrides(base, parseOverrides(process.argv.slice(2)));
  } catch (error) {
    process.stderr.write(`size: ${error instanceof Error ? error.message : String(error)}\n`);
    process.exit(1);
  }
};

const config = loadConfig();
const dist = fileURLToPath(new URL('../dist/', import.meta.url));
const html = readFileSync(join(dist, 'index.html'), 'utf8');

const STATIC_IMPORT =
  /(?:^|[^\w$.])(?:import|export)\s*(?:[\w$]+\s*,?\s*)?(?:\*\s*(?:as\s+[\w$]+\s*)?|\{[^{}]*\}\s*)?(?:from\s*)?["']([^"']+)["']/g;

const attr = (tag, name) => new RegExp(`\\s${name}="([^"]*)"`, 'i').exec(tag)?.[1];

const fail = (message) => {
  process.stderr.write(`size: ${message}\n`);
  process.exit(1);
};

const checkInline = () => {
  for (const [tag] of html.matchAll(/<script\b[^>]*>/gi)) {
    if (!attr(tag, 'src')) fail(`dist/index.html has an inline script, which the 'self' CSP blocks: ${tag}`);
  }
  if (/<style\b/i.test(html)) fail(`dist/index.html has an inline <style>, which the 'self' CSP blocks`);
  if (/\sstyle="/i.test(html)) fail(`dist/index.html has a style attribute, which the 'self' CSP blocks`);
};

const entries = () => {
  const js = new Set();
  const css = new Set();
  for (const [tag] of html.matchAll(/<script\b[^>]*>/gi)) {
    if (attr(tag, 'type') === 'module' && attr(tag, 'src')) js.add(attr(tag, 'src'));
  }
  for (const [tag] of html.matchAll(/<link\b[^>]*>/gi)) {
    const rel = attr(tag, 'rel');
    if (rel === 'modulepreload') js.add(attr(tag, 'href'));
    if (rel === 'stylesheet') css.add(attr(tag, 'href'));
  }
  return { js, css };
};

const fileOf = (ref) => join(dist, ref.replace(/^\//, ''));

const staticImports = (ref) => {
  const code = readFileSync(fileOf(ref), 'utf8');
  const found = [];
  for (const [, spec] of code.matchAll(STATIC_IMPORT)) {
    if (!/^\.{0,2}\//.test(spec)) continue;
    const target = spec.startsWith('/') ? spec : posix.join(posix.dirname(ref), spec);
    if (!existsSync(fileOf(target))) fail(`${ref} imports ${spec}, which is not in dist/`);
    found.push(target);
  }
  return found;
};

const followGraph = (roots) => {
  const seen = new Set();
  const queue = [...roots];
  while (queue.length > 0) {
    const ref = queue.shift();
    if (seen.has(ref)) continue;
    seen.add(ref);
    queue.push(...staticImports(ref));
  }
  return seen;
};

const gzipSize = (ref) => gzipSync(readFileSync(fileOf(ref)), { level: 9 }).length;

const kb = (bytes) => (bytes / KB).toFixed(1);

const sized = (refs) => [...refs].map((ref) => [ref, gzipSize(ref)]).sort((a, b) => b[1] - a[1]);

const allScripts = () =>
  readdirSync(join(dist, 'assets'))
    .filter((file) => file.endsWith('.js'))
    .map((file) => `/assets/${file}`);

const printRows = (title, rows) => {
  process.stdout.write(`${title}\n`);
  for (const [ref, bytes] of rows) process.stdout.write(`${kb(bytes).padStart(7)} KB  ${ref}\n`);
};

checkInline();
const roots = entries();
const initialJs = followGraph(roots.js);
if (initialJs.size === 0) fail('no initial js found from dist/index.html');
if (roots.css.size === 0) fail('no initial css found from dist/index.html');
const report = evaluate(
  {
    InitialJs: sized(initialJs),
    InitialCss: sized(roots.css),
    Lazy: sized(allScripts().filter((ref) => !initialJs.has(ref))),
  },
  config,
);

printRows('initial js', sized(initialJs));
printRows('initial css', sized(roots.css));
process.stdout.write('lazy js (2 KB and over, and any over budget)\n');
for (const chunk of report.Lazy.filter((entry) => entry.Over || entry.Bytes >= 2 * KB)) {
  const verdict = chunk.Over ? ` OVER ${kb(chunk.Budget)} KB` : '';
  process.stdout.write(`${kb(chunk.Bytes).padStart(7)} KB  ${chunk.Ref}${verdict}\n`);
}
for (const rule of overrideMatches(report.Lazy.map((chunk) => chunk.Name), config)) {
  process.stdout.write(`size: lazy override ${rule.Pattern} (${rule.Kb} KB) matches ${rule.Matches} chunks\n`);
}
for (const row of report.Initial) {
  const verdict = row.Over ? 'OVER' : 'ok';
  process.stdout.write(`size: initial ${row.Kind} is ${kb(row.Bytes)} KB gzip, budget ${kb(row.Budget)} KB: ${verdict}\n`);
}
const lazyOver = report.Lazy.filter((chunk) => chunk.Over);
process.stdout.write(
  `size: ${report.Lazy.length} lazy chunks, ${lazyOver.length} over their budget (default ${config.LazyJsKb} KB): ${lazyOver.length === 0 ? 'ok' : 'OVER'}\n`,
);

if (report.Failed) process.exit(1);

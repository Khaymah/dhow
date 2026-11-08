import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

import { assertParity, buildContent, renderContent, renderMeta, RELEASES_FOLDER, VERSION } from './HelpBuild.mjs';
import { HelpError } from './HelpMarkdown.mjs';

const web = join(import.meta.dirname, '..');
export const DOCS_ROOT = join(web, '..', '..', 'docs');
export const OUTPUT = join(web, 'Src', 'Core', 'Help', 'Generated');
const FOLDERS = { ar: '', en: 'en' };

const releaseVersions = (docsRoot) => {
  const lists = Object.values(FOLDERS).map((folder) =>
    readdirSync(join(docsRoot, folder, RELEASES_FOLDER))
      .filter((name) => name.endsWith('.md'))
      .map((name) => name.slice(0, -3))
      .sort(),
  );
  const [ar, en] = lists;
  if (JSON.stringify(ar) !== JSON.stringify(en)) {
    throw new HelpError('docs/releases', 0, `docs/releases and docs/en/releases must hold the same versions: ${ar.join(', ')} against ${en.join(', ')}`);
  }
  for (const version of ar) {
    if (!VERSION.test(version)) throw new HelpError(`docs/releases/${version}.md`, 0, 'the file name must be X.Y.Z.md or X.Y.Z-rc.N.md');
  }
  if (ar.length === 0) throw new HelpError('docs/releases', 0, 'there must be at least one release notes file');
  return ar;
};

export const generate = (docsRoot = DOCS_ROOT) => {
  const read = (locale, rel) => readFileSync(join(docsRoot, FOLDERS[locale], rel), 'utf8');
  const locate = (rel) => existsSync(join(docsRoot, rel)) && existsSync(join(docsRoot, 'en', rel));
  const versions = releaseVersions(docsRoot);
  const ar = buildContent({ locale: 'ar', read, locate, versions });
  const en = buildContent({ locale: 'en', read, locate, versions });
  assertParity(ar, en);
  return {
    'HelpContentAr.ts': renderContent('HelpContentAr', ar),
    'HelpContentEn.ts': renderContent('HelpContentEn', en),
    'ReleaseMeta.ts': renderMeta(ar.Releases[0].Version),
  };
};

const main = () => {
  try {
    const files = generate();
    mkdirSync(OUTPUT, { recursive: true });
    for (const [name, text] of Object.entries(files)) writeFileSync(join(OUTPUT, name), text);
  } catch (error) {
    if (!(error instanceof HelpError)) throw error;
    process.stderr.write(`gen:help: ${error.message}\n`);
    process.exit(1);
  }
};

if (process.argv[1] && dirname(process.argv[1]) === import.meta.dirname) main();

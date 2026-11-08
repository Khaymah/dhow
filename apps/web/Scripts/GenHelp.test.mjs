import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { DOCS_ROOT, generate, OUTPUT } from './GenHelp.mjs';
import { HelpError } from './HelpMarkdown.mjs';

const copies = [];
const copyDocs = () => {
  const root = mkdtempSync(join(tmpdir(), 'dhow-help-'));
  copies.push(root);
  cpSync(DOCS_ROOT, root, { recursive: true });
  return root;
};

afterEach(() => {
  for (const root of copies.splice(0)) rmSync(root, { recursive: true, force: true });
});

describe('the committed help files', () => {
  it('are what the documentation generates today, so run pnpm gen:help after editing docs', () => {
    for (const [name, text] of Object.entries(generate())) {
      expect(readFileSync(join(OUTPUT, name), 'utf8'), name).toBe(text);
    }
  });
});

describe('generate', () => {
  it('builds the same shape in both languages and exposes the latest release', () => {
    const files = generate();
    expect(Object.keys(files).sort()).toEqual(['HelpContentAr.ts', 'HelpContentEn.ts', 'ReleaseMeta.ts']);
    expect(files['ReleaseMeta.ts']).toMatch(/^export const LATEST_RELEASE = "\d+\.\d+\.\d+/);
  });

  it('refuses raw HTML added to the FAQ', () => {
    const root = copyDocs();
    const faq = join(root, 'faq.md');
    writeFileSync(faq, `${readFileSync(faq, 'utf8')}\n<script>alert(1)</script>\n`);
    expect(() => generate(root)).toThrow(/raw HTML is not allowed/);
  });

  it('refuses a javascript link added to the English FAQ', () => {
    const root = copyDocs();
    const faq = join(root, 'en', 'faq.md');
    writeFileSync(faq, `${readFileSync(faq, 'utf8')}\n[click](javascript:alert(1))\n`);
    expect(() => generate(root)).toThrow(/docs\/en\/faq\.md:\d+: link/);
  });

  it('refuses a data link in the release notes', () => {
    const root = copyDocs();
    const notes = join(root, 'releases', '0.1.0.md');
    writeFileSync(notes, `${readFileSync(notes, 'utf8')}\n- [x](data:text/html,hi)\n`);
    expect(() => generate(root)).toThrow(HelpError);
  });

  it('refuses a link to a page that exists in only one language', () => {
    const root = copyDocs();
    writeFileSync(join(root, 'only-arabic.md'), '# x\n');
    const faq = join(root, 'faq.md');
    writeFileSync(faq, readFileSync(faq, 'utf8').replace('(guides/secrets.md)', '(only-arabic.md)'));
    expect(() => generate(root)).toThrow(/missing from docs\/ or docs\/en\//);
  });

  it('fails when the two languages have different questions', () => {
    const root = copyDocs();
    const faq = join(root, 'en', 'faq.md');
    writeFileSync(faq, `${readFileSync(faq, 'utf8')}\n### One more\n\nAnswer.\n`);
    expect(() => generate(root)).toThrow(/does not match the Arabic/);
  });

  it('fails when a release exists in one language only', () => {
    const root = copyDocs();
    writeFileSync(join(root, 'releases', '0.2.0.md'), '# x\n\n## أهم التغييرات\n\n- a\n');
    expect(() => generate(root)).toThrow(/must hold the same versions/);
  });

  it('fails on a release file whose name is not a version', () => {
    const root = copyDocs();
    cpSync(join(root, 'releases', '0.1.0.md'), join(root, 'releases', 'latest.md'));
    cpSync(join(root, 'en', 'releases', '0.1.0.md'), join(root, 'en', 'releases', 'latest.md'));
    expect(() => generate(root)).toThrow(/X\.Y\.Z/);
  });
});

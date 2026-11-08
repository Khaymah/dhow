import { describe, expect, it } from 'vitest';

import { buildFaq, buildRelease, buildTroubleshooting, sortReleases } from './HelpBuild.mjs';
import { HelpError, parseDocument, resolveLink, splitSections, toBody } from './HelpMarkdown.mjs';

const locate = (rel) => ['guides/jobs.md', 'install/reverse-proxy.md', 'install/troubleshooting.md', 'faq.md'].includes(rel);
const ctx = (sourceRel = 'faq.md') => ({ source: `docs/${sourceRel}`, sourceRel, locate, where: { source: `docs/${sourceRel}`, line: 1 } });
const parse = (text, sourceRel) => parseDocument(text, ctx(sourceRel));
const inline = (text) => parse(text)[0].Inline;

describe('inline content', () => {
  it('keeps text, code and bold', () => {
    expect(inline('Run `make check` and **wait**.')).toEqual([
      { Type: 'text', Text: 'Run ' },
      { Type: 'code', Text: 'make check' },
      { Type: 'text', Text: ' and ' },
      { Type: 'strong', Text: 'wait' },
      { Type: 'text', Text: '.' },
    ]);
  });

  it('turns a relative link into a docs path and keeps an Arabic anchor', () => {
    expect(inline('[jobs](guides/jobs.md#قسم)')).toEqual([
      { Type: 'link', Kind: 'doc', Text: 'jobs', Doc: 'guides/jobs.md#قسم' },
    ]);
  });

  it('resolves a link against the folder of the source page', () => {
    const nodes = parseDocument('[proxy](reverse-proxy.md)', ctx('install/troubleshooting.md'))[0].Inline;
    expect(nodes[0]).toMatchObject({ Kind: 'doc', Doc: 'install/reverse-proxy.md' });
    const up = parseDocument('[jobs](../guides/jobs.md)', ctx('install/troubleshooting.md'))[0].Inline;
    expect(up[0]).toMatchObject({ Doc: 'guides/jobs.md' });
  });

  it('resolves a link that only has an anchor to the page itself', () => {
    const nodes = parseDocument('[up](#top)', ctx('install/troubleshooting.md'))[0].Inline;
    expect(nodes[0]).toMatchObject({ Doc: 'install/troubleshooting.md#top' });
  });

  it('allows an https link to the Dhow repository only', () => {
    expect(inline('[repo](https://github.com/Khaymah/dhow/issues)')[0]).toEqual({
      Type: 'link',
      Kind: 'external',
      Text: 'repo',
      Href: 'https://github.com/Khaymah/dhow/issues',
    });
  });

  it('reads an escaped character as text', () => {
    expect(inline('a \\*b\\* c')).toEqual([{ Type: 'text', Text: 'a *b* c' }]);
  });
});

describe('what the generator refuses', () => {
  it.each([
    ['raw HTML', '<b>bold</b>'],
    ['an HTML comment', '<!-- note -->'],
    ['a raw tag inside a sentence', 'Press <kbd>Enter</kbd> now.'],
    ['an autolink', '<https://example.com>'],
    ['an entity', 'Fish &amp; chips'],
    ['a javascript link', '[x](javascript:alert(1))'],
    ['a mixed-case javascript link', '[x](JaVaScRiPt:alert(1))'],
    ['a data link', '[x](data:text/html;base64,PHNjcmlwdD4=)'],
    ['an http link', '[x](http://github.com/Khaymah/dhow)'],
    ['a link to another host', '[x](https://evil.example/Khaymah/dhow)'],
    ['a host that only starts like ours', '[x](https://github.com/Khaymah/dhow.evil.example/a)'],
    ['a protocol-relative link', '[x](//evil.example/a.md)'],
    ['a mailto link', '[x](mailto:a@example.com)'],
    ['a link with a space', '[x](guides/jobs.md "title")'],
    ['a link with a backslash', '[x](guides\\jobs.md)'],
    ['a link outside docs', '[x](../../apps/web/package.json)'],
    ['a link that climbs out of docs', '[x](../../../etc/passwd.md)'],
    ['a link to a page that does not exist', '[x](guides/missing.md)'],
    ['a link to a file that is not markdown', '[x](guides/jobs.html)'],
    ['an image', '![x](guides/jobs.md)'],
    ['a nested link', '[a [b](guides/jobs.md)](guides/jobs.md)'],
    ['a reference link', '[x][1]'],
    ['a reference definition', '[1]: https://github.com/Khaymah/dhow'],
    ['a table', '| a | b |\n|---|---|\n| 1 | 2 |'],
    ['a blockquote', '> quoted'],
    ['a horizontal rule', '---'],
    ['an indented code block', '    code'],
    ['a tilde fence', '~~~\ncode\n~~~'],
    ['an unclosed code fence', '```\ncode'],
    ['an unclosed code span', 'a `b'],
    ['unclosed bold', 'a **b'],
    ['a nested list', '- a\n  - b'],
    ['a mixed list', '- a\n1. b'],
  ])('refuses %s', (_name, text) => {
    expect(() => parse(text)).toThrow(HelpError);
  });

  it('names the file and the line', () => {
    expect(() => parse('fine\n\n<script>alert(1)</script>')).toThrow(/^docs\/faq\.md:3: /);
  });

  it('does not look inside a code block for HTML', () => {
    const [block] = parse('```html\n<script>alert(1)</script>\n```');
    expect(block).toMatchObject({ Type: 'code', Text: '<script>alert(1)</script>' });
  });

  it('does not look inside a code span for HTML', () => {
    expect(inline('use `<div>`')[1]).toEqual({ Type: 'code', Text: '<div>' });
  });

  it('refuses a link from the resolver directly', () => {
    const where = { source: 'docs/faq.md', line: 1 };
    expect(() => resolveLink('javascript:alert(1)', 'faq.md', locate, where)).toThrow(/allowlist/);
    expect(() => resolveLink('/guides/jobs.md', 'faq.md', locate, where)).toThrow(HelpError);
    expect(() => resolveLink('guides/jobs.md#a b', 'faq.md', locate, where)).toThrow(HelpError);
  });
});

describe('blocks', () => {
  it('reads paragraphs, lists and code', () => {
    const blocks = toBody(parse('First line\ncontinues.\n\n- one\n- two\n  more\n\n1. a\n2. b\n\n```sh\nls\n```'));
    expect(blocks.map((block) => block.Type)).toEqual(['paragraph', 'list', 'list', 'code']);
    expect(blocks[1]).toMatchObject({ Ordered: false });
    expect(blocks[1].Items[1]).toEqual([{ Type: 'text', Text: 'two more' }]);
    expect(blocks[2]).toMatchObject({ Ordered: true });
  });

  it('skips the language switch link on the first line', () => {
    expect(parse('[English](en/faq.md)\n\n# Title')).toHaveLength(1);
  });
});

const faq = (extra = '') => `# Title\n\nIntro that is not shown.\n\n## Group\n\n### Question\n\nAnswer.\n${extra}`;

describe('the FAQ shape', () => {
  it('builds groups, questions and ids', () => {
    const groups = buildFaq(parse(faq('\n#### Detail\n\nMore.')), 'docs/faq.md');
    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({ Id: 'g1', Title: 'Group' });
    expect(groups[0].Entries[0].Id).toBe('g1-q1');
    expect(groups[0].Entries[0].Body.map((block) => block.Type)).toEqual(['paragraph', 'heading', 'paragraph']);
  });

  it.each([
    ['a question with no group', '# T\n\n### Q\n\nA.'],
    ['a group with no questions', '# T\n\n## G'],
    ['a question with no answer', '# T\n\n## G\n\n### Q'],
    ['text straight under a group', '# T\n\n## G\n\nStray.\n\n### Q\n\nA.'],
    ['a level 5 heading', '# T\n\n## G\n\n### Q\n\n##### Deep\n\nA.'],
    ['no title', '## G\n\n### Q\n\nA.'],
  ])('refuses %s', (_name, text) => {
    expect(() => buildFaq(parse(text), 'docs/faq.md')).toThrow(HelpError);
  });
});

describe('troubleshooting sections', () => {
  const page = '# T\n\n## One\n\n| a |\n|---|\n\n## Two\n\nText with `code`.\n\n```sh\nfence\n## not a section\n```\n\n### Sub\n\nMore.\n\n## Three\n\nLast.';
  const section = (text, base) => parseDocument(text, ctx('install/troubleshooting.md'), base);

  it('parses only the chosen sections, so an unrelated table does not matter', () => {
    const entries = buildTroubleshooting(page, 'docs/x.md', section, [1, 2]);
    expect(entries.map((entry) => entry.Title)).toEqual(['Two', 'Three']);
    expect(entries[0].Body.map((block) => block.Type)).toEqual(['paragraph', 'code', 'heading', 'paragraph']);
  });

  it('refuses a chosen section that has a table', () => {
    expect(() => buildTroubleshooting(page, 'docs/x.md', section, [0])).toThrow(/tables are not allowed/);
  });

  it('reports the line in the whole file', () => {
    expect(() => buildTroubleshooting(page, 'docs/x.md', section, [0])).toThrow(/:5: /);
  });

  it('refuses a section that does not exist', () => {
    expect(() => buildTroubleshooting(page, 'docs/x.md', section, [9])).toThrow(HelpError);
  });

  it('finds sections outside code fences only', () => {
    expect(splitSections(page)).toHaveLength(3);
  });
});

const AR = '# داو\n\n## أهم التغييرات\n\n- شيء.\n\n## إجراءات مطلوبة\n\nافعل كذا.\n';

describe('release notes', () => {
  it('keeps the fixed sections in order', () => {
    const release = buildRelease(parse(AR, 'releases/1.0.0.md'), 'docs/releases/1.0.0.md', 'ar', '1.0.0');
    expect(release.Sections.map((s) => s.Kind)).toEqual(['highlights', 'actions']);
    expect(release.Version).toBe('1.0.0');
  });

  it.each([
    ['an unknown heading', '# T\n\n## Something else\n\n- a'],
    ['sections out of order', '# T\n\n## إجراءات مطلوبة\n\nx\n\n## أهم التغييرات\n\n- a'],
    ['a missing highlights section', '# T\n\n## إصلاحات أمنية\n\n- a'],
    ['text before the first section', '# T\n\nIntro\n\n## أهم التغييرات\n\n- a'],
    ['an empty section', '# T\n\n## أهم التغييرات'],
    ['English headings in the Arabic page', '# T\n\n## Highlights\n\n- a'],
  ])('refuses %s', (_name, text) => {
    expect(() => buildRelease(parse(text, 'releases/1.0.0.md'), 'docs/releases/1.0.0.md', 'ar', '1.0.0')).toThrow(HelpError);
  });

  it('sorts the newest version first and puts a release candidate before its release', () => {
    const versions = ['0.9.0', '1.0.0-rc.1', '1.0.0', '1.0.0-rc.2', '0.10.0'].map((Version) => ({ Version }));
    expect(sortReleases(versions).map((v) => v.Version)).toEqual(['1.0.0', '1.0.0-rc.2', '1.0.0-rc.1', '0.10.0', '0.9.0']);
  });
});

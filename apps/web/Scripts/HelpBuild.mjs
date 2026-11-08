import { HelpError, parseDocument, splitSections, toBody } from './HelpMarkdown.mjs';

export const RELEASE_SECTIONS = {
  ar: [
    ['highlights', 'أهم التغييرات'],
    ['security', 'إصلاحات أمنية'],
    ['actions', 'إجراءات مطلوبة'],
    ['deprecations', 'ما سيتوقف لاحقًا'],
  ],
  en: [
    ['highlights', 'Highlights'],
    ['security', 'Security fixes'],
    ['actions', 'Actions required'],
    ['deprecations', 'Deprecations'],
  ],
};

export const TROUBLESHOOTING_SOURCE = 'install/troubleshooting.md';
export const TROUBLESHOOTING_SECTIONS = [2, 3, 4, 5, 6];
export const FAQ_SOURCE = 'faq.md';
export const RELEASES_FOLDER = 'releases';
export const VERSION = /^(\d{1,6})\.(\d{1,6})\.(\d{1,6})(?:-rc\.(\d{1,6}))?$/;

const fail = (source, line, message) => new HelpError(source, line, message);

const takeTitle = (blocks, source) => {
  const [first, ...rest] = blocks;
  if (first?.Type !== 'heading' || first.Level !== 1) throw fail(source, first?.Line ?? 1, 'the page must start with one # heading');
  return { title: first.Text, rest };
};

export const buildFaq = (blocks, source) => {
  const { rest } = takeTitle(blocks, source);
  const groups = [];
  let entry = null;
  for (const block of rest) {
    const group = groups.at(-1);
    if (block.Type === 'heading' && block.Level === 2) {
      groups.push({ Id: `g${String(groups.length + 1)}`, Title: block.Text, Entries: [] });
      entry = null;
    } else if (block.Type === 'heading' && block.Level === 3) {
      if (!group) throw fail(source, block.Line, 'a ### question needs a ## group before it');
      entry = { Id: `${group.Id}-q${String(group.Entries.length + 1)}`, Title: block.Text, Body: [] };
      group.Entries.push(entry);
    } else if (block.Type === 'heading' && block.Level !== 4) {
      throw fail(source, block.Line, `a level ${String(block.Level)} heading is not allowed here`);
    } else if (entry) {
      entry.Body.push(block);
    } else if (group) {
      throw fail(source, block.Line, 'text under a ## group must be inside a ### question');
    }
  }
  for (const group of groups) {
    if (group.Entries.length === 0) throw fail(source, 1, `the group "${group.Title}" has no questions`);
    for (const item of group.Entries) {
      if (item.Body.length === 0) throw fail(source, 1, `the question "${item.Title}" has no answer`);
      item.Body = toBody(item.Body);
    }
  }
  if (groups.length === 0) throw fail(source, 1, 'the FAQ has no ## groups');
  return groups;
};

const sections = (blocks) => {
  const out = [];
  for (const block of blocks) {
    if (block.Type === 'heading' && block.Level === 2) out.push({ heading: block, blocks: [] });
    else out.at(-1)?.blocks.push(block);
  }
  return out;
};

const lowerHeadings = (blocks, source) =>
  blocks.map((block) => {
    if (block.Type !== 'heading') return block;
    if (block.Level < 3 || block.Level > 4) throw fail(source, block.Line, `a level ${String(block.Level)} heading is not allowed here`);
    return block;
  });

export const buildTroubleshooting = (markdown, source, parseSection, picks = TROUBLESHOOTING_SECTIONS) => {
  const all = splitSections(markdown);
  return picks.map((ordinal, index) => {
    const found = all[ordinal];
    if (!found) throw fail(source, 1, `the page has no section number ${String(ordinal + 1)}`);
    const [heading, ...rest] = parseSection(found.lines.join('\n'), found.line - 1);
    const body = toBody(lowerHeadings(rest, source));
    if (body.length === 0) throw fail(source, found.line, 'the section is empty');
    return { Id: `t${String(index + 1)}`, Title: heading.Text, Body: body };
  });
};

export const buildRelease = (blocks, source, locale, version) => {
  const { title, rest } = takeTitle(blocks, source);
  const expected = RELEASE_SECTIONS[locale];
  const found = sections(rest);
  const firstSection = rest.findIndex((block) => block.Type === 'heading' && block.Level === 2);
  if (firstSection === -1) throw fail(source, 1, 'the release notes have no ## sections');
  if (firstSection > 0) throw fail(source, rest[0].Line, 'text before the first ## section is not allowed');
  let cursor = 0;
  const out = found.map(({ heading, blocks: body }) => {
    const at = expected.findIndex(([, name], index) => index >= cursor && name === heading.Text);
    if (at === -1) {
      const names = expected.map(([, name]) => name).join(' / ');
      throw fail(source, heading.Line, `"${heading.Text}" is not an allowed section here; the fixed order is: ${names}`);
    }
    cursor = at + 1;
    const content = toBody(lowerHeadings(body, source));
    if (content.length === 0) throw fail(source, heading.Line, 'the section is empty');
    return { Kind: expected[at][0], Title: heading.Text, Body: content };
  });
  if (out[0]?.Kind !== 'highlights') throw fail(source, 1, `the first section must be "${expected[0][1]}"`);
  return { Version: version, Title: title, Sections: out };
};

const compareVersions = (a, b) => {
  const [pa, pb] = [VERSION.exec(a), VERSION.exec(b)];
  for (let i = 1; i <= 3; i += 1) {
    if (pa[i] !== pb[i]) return Number(pb[i]) - Number(pa[i]);
  }
  if (pa[4] === undefined && pb[4] === undefined) return 0;
  if (pa[4] === undefined) return -1;
  if (pb[4] === undefined) return 1;
  return Number(pb[4]) - Number(pa[4]);
};

export const sortReleases = (releases) => [...releases].sort((a, b) => compareVersions(a.Version, b.Version));

const shape = (content) => ({
  faq: content.Faq.map((group) => group.Entries.length),
  troubleshooting: content.Troubleshooting.length,
  releases: content.Releases.map((release) => `${release.Version}:${release.Sections.map((s) => s.Kind).join(',')}`),
});

export const assertParity = (ar, en) => {
  const [a, b] = [JSON.stringify(shape(ar)), JSON.stringify(shape(en))];
  if (a !== b) throw new HelpError('docs/en', 0, `the English help content does not match the Arabic one: ${a} against ${b}`);
};

export const buildContent = ({ locale, read, locate, versions }) => {
  const ctx = (source) => ({ source: `${locale === 'ar' ? 'docs' : 'docs/en'}/${source}`, sourceRel: source, locate, where: { line: 1 } });
  const parse = (rel) => parseDocument(read(locale, rel), ctx(rel));
  const releases = versions.map((version) => {
    const rel = `${RELEASES_FOLDER}/${version}.md`;
    return buildRelease(parse(rel), ctx(rel).source, locale, version);
  });
  return {
    Faq: buildFaq(parse(FAQ_SOURCE), ctx(FAQ_SOURCE).source),
    Troubleshooting: buildTroubleshooting(
      read(locale, TROUBLESHOOTING_SOURCE),
      ctx(TROUBLESHOOTING_SOURCE).source,
      (text, base) => parseDocument(text, ctx(TROUBLESHOOTING_SOURCE), base),
    ),
    Releases: sortReleases(releases),
  };
};

export const renderContent = (name, content) =>
  `import type { IHelpContent } from '@/Core/Help/HelpTypes';\n\nexport const ${name}: IHelpContent = ${JSON.stringify(content, null, 2)};\n`;

export const renderMeta = (latest) => `export const LATEST_RELEASE = ${JSON.stringify(latest)};\n`;

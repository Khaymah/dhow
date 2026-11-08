import { posix } from 'node:path';

export class HelpError extends Error {
  constructor(source, line, message) {
    super(`${source}:${String(line)}: ${message}`);
    this.name = 'HelpError';
  }
}

export const EXTERNAL_PREFIX = 'https://github.com/Khaymah/dhow';

const DOC_PATH = /^[a-z0-9][a-z0-9/_-]*\.md$/;
const DOC_ANCHOR = /^[^\s#<>"'\\]+$/u;
const hasControl = (text) => Array.from(text).some((char) => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127);
const EXTERNAL_URL = /^https:\/\/github\.com\/Khaymah\/dhow(?:[/?#][A-Za-z0-9._~:/?#@!$&*+,;=%-]*)?$/;
const SCHEME = /^[A-Za-z][A-Za-z0-9+.-]*:/;
const FENCE = /^```([A-Za-z0-9_-]*)\s*$/;
const HEADING = /^(#{1,6}) +(\S.*)$/;
const ITEM = /^( *)(?:([-*])|(\d{1,3})\.) +(\S.*)$/;
const ENTITY = /&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);/;
const SWITCH_LINK = /^\[(?:English|العربية)\]\([^)\s]+\)$/u;

export const resolveLink = (url, sourceRel, locate, where) => {
  const fail = (why) => new HelpError(where.source, where.line, `link ${JSON.stringify(url)} ${why}`);
  if (url === '' || hasControl(url) || /[\s\\<>"']/u.test(url)) throw fail('contains a character that is not allowed');
  if (url.startsWith('//')) throw fail('is protocol-relative');
  if (url.startsWith('/')) throw fail('is an absolute path');
  if (SCHEME.test(url)) {
    if (!EXTERNAL_URL.test(url)) throw fail(`has a scheme or host outside the allowlist (${EXTERNAL_PREFIX})`);
    return { Type: 'external', Href: url };
  }
  const hash = url.indexOf('#');
  const file = hash === -1 ? url : url.slice(0, hash);
  const anchor = hash === -1 ? '' : url.slice(hash + 1);
  if (hash !== -1 && (hasControl(anchor) || !DOC_ANCHOR.test(anchor))) throw fail('has an invalid anchor');
  const target = file === '' ? sourceRel : posix.normalize(posix.join(posix.dirname(sourceRel), file));
  if (target.startsWith('..') || target.startsWith('/') || !DOC_PATH.test(target)) throw fail('is not a docs page');
  if (!locate(target)) throw fail(`points to ${target}, which is missing from docs/ or docs/en/`);
  return { Type: 'doc', Doc: anchor === '' ? target : `${target}#${anchor}` };
};

const escaped = /\\([!-/:-@[-`{-~])/g;
const unescape = (text) => text.replace(escaped, '$1');

const closing = (text, from, token, where) => {
  const at = text.indexOf(token, from);
  if (at === -1) throw new HelpError(where.source, where.line, `unclosed ${token} in ${JSON.stringify(text)}`);
  return at;
};

const flatten = (nodes) => nodes.map((node) => node.Text).join('');

const linkAt = (text, start, ctx) => {
  const close = closing(text, start + 1, ']', ctx.where);
  if (text[close + 1] !== '(') throw new HelpError(ctx.where.source, ctx.where.line, 'only inline links [text](url) are allowed');
  const end = closing(text, close + 2, ')', ctx.where);
  const label = parseInline(text.slice(start + 1, close), { ...ctx, inLink: true });
  const target = resolveLink(text.slice(close + 2, end), ctx.sourceRel, ctx.locate, ctx.where);
  return { node: { ...target, Type: 'link', Kind: target.Type, Text: flatten(label) }, next: end + 1 };
};

const pushText = (nodes, text) => {
  if (text === '') return;
  const last = nodes.at(-1);
  if (last?.Type === 'text') last.Text += text;
  else nodes.push({ Type: 'text', Text: text });
};

const rejectRaw = (text, ctx) => {
  if (text.includes('<')) throw new HelpError(ctx.where.source, ctx.where.line, 'raw HTML is not allowed');
  if (ENTITY.test(text)) throw new HelpError(ctx.where.source, ctx.where.line, 'HTML entities are not allowed');
};

export const parseInline = (text, ctx) => {
  const nodes = [];
  let plain = '';
  const flush = () => {
    rejectRaw(plain, ctx);
    pushText(nodes, unescape(plain));
    plain = '';
  };
  for (let i = 0; i < text.length; ) {
    const ch = text[i];
    if (ch === '\\' && i + 1 < text.length) {
      plain += text.slice(i, i + 2);
      i += 2;
    } else if (ch === '`') {
      const ticks = /^`+/.exec(text.slice(i))[0];
      const end = closing(text, i + ticks.length, ticks, ctx.where);
      flush();
      nodes.push({ Type: 'code', Text: text.slice(i + ticks.length, end).trim() });
      i = end + ticks.length;
    } else if (ch === '!' && text[i + 1] === '[') {
      throw new HelpError(ctx.where.source, ctx.where.line, 'images are not allowed');
    } else if (ch === '[') {
      if (ctx.inLink) throw new HelpError(ctx.where.source, ctx.where.line, 'links cannot be nested');
      flush();
      const { node, next } = linkAt(text, i, ctx);
      nodes.push(node);
      i = next;
    } else if (ch === '*' && text[i + 1] === '*') {
      const end = closing(text, i + 2, '**', ctx.where);
      flush();
      nodes.push({ Type: 'strong', Text: flatten(parseInline(text.slice(i + 2, end), { ...ctx, inLink: true })) });
      i = end + 2;
    } else {
      plain += ch;
      i += 1;
    }
  }
  flush();
  return nodes;
};

const lineAt = (ctx, index) => ({ source: ctx.source, line: (ctx.lineBase ?? 0) + index + 1 });

const rejectLine = (line, ctx) => {
  const reasons = [
    [/^\|/, 'tables are not allowed'],
    [/^>/, 'blockquotes are not allowed'],
    [/^(?:-{3,}|\*{3,}|_{3,})$/, 'horizontal rules are not allowed'],
    [/^\[[^\]]+\]:\s/, 'link reference definitions are not allowed'],
    [/^~~~/, 'only ``` fences are allowed'],
    [/^ {4,}\S/, 'indented code blocks are not allowed'],
  ];
  for (const [pattern, why] of reasons) {
    if (pattern.test(line)) throw new HelpError(ctx.where.source, ctx.where.line, why);
  }
};

const readFence = (lines, start, ctx) => {
  const body = [];
  for (let i = start + 1; i < lines.length; i += 1) {
    if (lines[i].trim() === '```') return { block: { Type: 'code', Text: body.join('\n') }, next: i + 1 };
    body.push(lines[i]);
  }
  throw new HelpError(ctx.source, lineAt(ctx, start).line, 'unclosed code fence');
};

const readList = (lines, start, ctx) => {
  const items = [];
  const first = ITEM.exec(lines[start]);
  const ordered = first[3] !== undefined;
  let i = start;
  while (i < lines.length) {
    const match = ITEM.exec(lines[i]);
    if (!match) break;
    if (match[1] !== '') throw new HelpError(ctx.source, lineAt(ctx, i).line, 'nested lists are not allowed');
    if ((match[3] !== undefined) !== ordered) throw new HelpError(ctx.source, lineAt(ctx, i).line, 'a list cannot mix bullets and numbers');
    const parts = [match[4]];
    const at = i + 1;
    i += 1;
    while (i < lines.length && /^ {2,}\S/.test(lines[i]) && !ITEM.test(lines[i])) {
      parts.push(lines[i].trim());
      i += 1;
    }
    if (i < lines.length && /^ +(?:[-*]|\d+\.) /.test(lines[i])) throw new HelpError(ctx.source, lineAt(ctx, i).line, 'nested lists are not allowed');
    items.push(parseInline(parts.join(' '), { ...ctx, where: lineAt(ctx, at - 1) }));
  }
  return { block: { Type: 'list', Ordered: ordered, Items: items }, next: i };
};

const BLOCK_START = (line) => HEADING.test(line) || FENCE.test(line) || ITEM.test(line) || line.trim() === '';

const readParagraph = (lines, start, ctx) => {
  const parts = [];
  let i = start;
  while (i < lines.length && !BLOCK_START(lines[i])) {
    rejectLine(lines[i], { ...ctx, where: lineAt(ctx, i) });
    parts.push(lines[i].trim());
    i += 1;
  }
  const where = lineAt(ctx, start);
  return { block: { Type: 'paragraph', Inline: parseInline(parts.join(' '), { ...ctx, where }) }, next: i };
};

export const parseDocument = (markdown, base, lineBase = 0) => {
  const ctx = { ...base, lineBase };
  const lines = markdown.replace(/\r\n?/g, '\n').split('\n');
  const out = [];
  let i = SWITCH_LINK.test(lines[0] ?? '') ? 1 : 0;
  while (i < lines.length) {
    const line = lines[i];
    const where = lineAt(ctx, i);
    const heading = HEADING.exec(line);
    if (line.trim() === '') {
      i += 1;
    } else if (FENCE.test(line)) {
      const { block, next } = readFence(lines, i, ctx);
      out.push({ ...block, Line: lineBase + i + 1 });
      i = next;
    } else if (heading) {
      const text = flatten(parseInline(heading[2].trim(), { ...ctx, where, inLink: true }));
      out.push({ Type: 'heading', Level: heading[1].length, Text: text, Line: lineBase + i + 1 });
      i += 1;
    } else if (ITEM.test(line)) {
      const { block, next } = readList(lines, i, ctx);
      out.push({ ...block, Line: lineBase + i + 1 });
      i = next;
    } else {
      const { block, next } = readParagraph(lines, i, ctx);
      out.push({ ...block, Line: lineBase + i + 1 });
      i = next;
    }
  }
  return out;
};

export const toBody = (blocks) =>
  blocks.map((block) => {
    if (block.Type === 'heading') return { Type: 'heading', Text: block.Text };
    if (block.Type === 'code') return { Type: 'code', Text: block.Text };
    if (block.Type === 'list') return { Type: 'list', Ordered: block.Ordered, Items: block.Items };
    return { Type: 'paragraph', Inline: block.Inline };
  });

export const splitSections = (markdown) => {
  const sections = [];
  let fenced = false;
  markdown
    .replace(/\r\n?/g, '\n')
    .split('\n')
    .forEach((line, index) => {
      if (FENCE.test(line)) fenced = !fenced;
      const match = fenced ? null : /^## +(\S.*)$/.exec(line);
      if (match) sections.push({ line: index + 1, lines: [line] });
      else sections.at(-1)?.lines.push(line);
    });
  return sections;
};

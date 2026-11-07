import { describe, expect, it } from 'vitest';

import { BIDI_CLASS, parseAnsi, PLAIN_STYLE } from '@/Modules/Jobs/Types/Ansi';

const ESC = '\u001b';

describe('parseAnsi', () => {
  it.each([
    ['plain text', 'hello', [{ Text: 'hello', ClassName: '' }]],
    ['a red word', `a ${ESC}[31mred${ESC}[0m b`, [{ Text: 'a ', ClassName: '' }, { Text: 'red', ClassName: 'text-accent-coral' }, { Text: ' b', ClassName: '' }]],
    ['bold bright green', `${ESC}[1;92mok`, [{ Text: 'ok', ClassName: 'text-accent-green font-semibold' }]],
    ['256 colors are dropped, not misread', `${ESC}[38;5;196mx${ESC}[39my`, [{ Text: 'xy', ClassName: '' }]],
    ['the first 16 of the 256 colors use the basic classes', `${ESC}[38;5;9mx${ESC}[0m`, [{ Text: 'x', ClassName: 'text-accent-coral' }]],
    ['true colors are dropped', `${ESC}[38;2;255;0;0mx`, [{ Text: 'x', ClassName: '' }]],
    ['colon sub-parameters are ignored, the rest still applies', `${ESC}[1;38:5:196mx`, [{ Text: 'x', ClassName: 'font-semibold' }]],
    ['background colors are ignored', `${ESC}[41mx`, [{ Text: 'x', ClassName: '' }]],
    ['cursor moves and titles are removed', `${ESC}[2K${ESC}]0;title\u0007${ESC}[1Gdone`, [{ Text: 'done', ClassName: '' }]],
    ['other control characters are removed, tabs kept', 'a\u0000\tb\u0008', [{ Text: 'a\tb', ClassName: '' }]],
  ])('handles %s', (_name, input, expected) => {
    expect(parseAnsi(input).Segments).toEqual(expected);
  });

  it('carries the style to the next call and resets with a bare SGR', () => {
    const first = parseAnsi(`${ESC}[33mwarn`);
    expect(first.Style.Fg).toBe('yellow');
    const second = parseAnsi(`still${ESC}[m plain`, first.Style);
    expect(second.Segments).toEqual([
      { Text: 'still', ClassName: 'text-accent-amber' },
      { Text: ' plain', ClassName: '' },
    ]);
    expect(second.Style).toEqual(PLAIN_STYLE);
  });

  it.each([
    ['an OSC 8 hyperlink with BEL', `a${ESC}]8;;https://evil.example/x\u0007link${ESC}]8;;\u0007b`, 'alinkb'],
    ['an OSC 8 hyperlink with ESC backslash', `a${ESC}]8;id=1;javascript:alert(1)${ESC}\\link${ESC}]8;;${ESC}\\b`, 'alinkb'],
    ['an OSC title that never ends', `a${ESC}]0;title without end`, 'a'],
    ['an OSC ended by the 8-bit terminator', `a${ESC}]0;t\u009cb`, 'ab'],
    ['a DCS string', `a${ESC}Pq#0;2;0;0;0${ESC}\\b`, 'ab'],
    ['an APC string', `a${ESC}_Gf=100;payload${ESC}\\b`, 'ab'],
    ['cursor movement and erase sequences', `a${ESC}[10A${ESC}[5;5H${ESC}[2J${ESC}[?25lb${ESC}[3 q`, 'ab'],
    ['8-bit control characters', 'a\u009b31mb\u009d0;t\u0007c\u0085', 'a31mb0;tc'],
    ['bell, backspace and null bytes', 'a\u0007\u0008\u0000b', 'ab'],
    ['markup', '<script>alert(1)</script><b>x</b>', '<script>alert(1)</script><b>x</b>'],
  ])('neutralizes %s', (_name, input, expected) => {
    const text = parseAnsi(input).Segments.map((segment) => segment.Text).join('');
    expect(text).toBe(expected);
  });

  it.each(['\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069', '\u200e', '\u200f', '\u061c'])(
    'shows the bidi control U+%#04x as a visible mark instead of reordering text',
    (control) => {
      const { Segments } = parseAnsi(`safe${control}tail`);
      const text = Segments.map((segment) => segment.Text).join('');
      expect(text).not.toContain(control);
      expect(text).toMatch(/^safe\u2039U\+[0-9A-F]{4}\u203atail$/);
      expect(Segments.some((segment) => segment.ClassName === BIDI_CLASS)).toBe(true);
    },
  );

  it('keeps the Arabic text and marks only the override', () => {
    const { Segments } = parseAnsi('خطأ \u202Eevil\u202C تم');
    expect(Segments.map((segment) => segment.Text).join('')).toBe('خطأ \u2039U+202E\u203aevil\u2039U+202C\u203a تم');
  });

  it('parses a one-megabyte line without sequences quickly and keeps every character', () => {
    const line = 'x'.repeat(1_000_000);
    const started = performance.now();
    const { Segments } = parseAnsi(line);
    expect(performance.now() - started).toBeLessThan(1000);
    expect(Segments).toEqual([{ Text: line, ClassName: '' }]);
  });

  it('parses long runs of unfinished escape sequences in linear time', () => {
    const line = `${ESC}[`.repeat(100_000) + `${ESC}]`.repeat(100_000);
    const started = performance.now();
    parseAnsi(line);
    expect(performance.now() - started).toBeLessThan(1000);
  });
});

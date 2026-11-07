import { describe, expect, it } from 'vitest';

const sources = import.meta.glob<string>(['../../App/**/*.tsx', '../../Core/Ui/**/*.tsx', '../../Modules/**/*.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
});

const files = Object.entries(sources).filter(([path]) => !path.endsWith('.test.tsx'));

const offenders = (pattern: RegExp, allow = /$^/): string[] =>
  files.filter(([path, source]) => pattern.test(source) && !allow.test(path)).map(([path]) => path);

describe('accessibility rules that hold in every component', () => {
  it('scans the shell, Core/Ui and the modules', () => {
    expect(files.some(([path]) => path.endsWith('IconButton.tsx'))).toBe(true);
    expect(files.some(([path]) => path.includes('/Modules/Secrets/'))).toBe(true);
    expect(files.some(([path]) => path.includes('/App/Layout/'))).toBe(true);
  });

  it('never removes the focus outline except on the palette input, whose box shows the focus', () => {
    expect(offenders(/(?<![\w-])(focus:)?outline-(none|0)(?![\w-])/, /CommandPalette\.tsx$/)).toEqual([]);
  });

  it('keeps every native checkbox target at 24px or more', () => {
    expect(offenders(/<input[^>]*type="checkbox"[^>]*(?<![\w-])size-[1-5](?![\w.-])/s)).toEqual([]);
    expect(offenders(/<input[^>]*(?<![\w-])size-[1-5](?![\w.-])[^>]*type="checkbox"/s)).toEqual([]);
  });

  it('never puts a click handler on a div or span without a role', () => {
    expect(offenders(/<(div|span)\b(?![^>]*role=)[^>]*onClick=/s)).toEqual([]);
  });

  it('always passes a label to an icon button', () => {
    expect(offenders(/<IconButton(?![^>]*label=)[^>]*\/?>/s, /IconButton\.tsx$/)).toEqual([]);
  });
});

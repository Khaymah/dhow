export type TAnsiColor = 'black' | 'red' | 'green' | 'yellow' | 'blue' | 'magenta' | 'cyan' | 'white';

export interface IAnsiStyle {
  Fg: TAnsiColor | null;
  Bold: boolean;
  Dim: boolean;
  Italic: boolean;
  Underline: boolean;
}

export interface IAnsiSegment {
  Text: string;
  ClassName: string;
}

export interface IAnsiResult {
  Segments: IAnsiSegment[];
  Style: IAnsiStyle;
}

export const PLAIN_STYLE: IAnsiStyle = { Fg: null, Bold: false, Dim: false, Italic: false, Underline: false };

const COLORS: readonly TAnsiColor[] = ['black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white'];

const COLOR_CLASS: Record<TAnsiColor, string> = {
  black: 'text-tertiary',
  red: 'text-accent-coral',
  green: 'text-accent-green',
  yellow: 'text-accent-amber',
  blue: 'text-accent-blue',
  magenta: 'text-accent-violet',
  cyan: 'text-accent-violet-soft',
  white: 'text-primary',
};

export const BIDI_CLASS = 'text-accent-amber';

const ESC = '\\u001b';
const BEL = '\\u0007';
const ST = '\\u009c';
const SEQUENCE = new RegExp(
  [
    `${ESC}\\[([0-?]*)([ -/]*)([@-~])`,
    `${ESC}\\][^${BEL}${ESC}${ST}]*(?:${BEL}|${ESC}\\\\|${ST})?`,
    `${ESC}[PX^_][^${ESC}${ST}]*(?:${ESC}\\\\|${ST})?`,
    `${ESC}[@-Z\\\\-_]?`,
    '[\\u0000-\\u0008\\u000b-\\u001f\\u007f-\\u009f]',
  ].join('|'),
  'g',
);
const BIDI_CONTROLS = /[؜‎‏‪-‮⁦-⁩]/gu;
const SGR_PARAMS = /^[0-9;:]*$/;

export const bidiLabel = (character: string): string =>
  `‹U+${(character.codePointAt(0) ?? 0).toString(16).toUpperCase().padStart(4, '0')}›`;

export const classOf = (style: IAnsiStyle): string =>
  [
    style.Fg === null ? '' : COLOR_CLASS[style.Fg],
    style.Bold ? 'font-semibold' : '',
    style.Dim ? 'opacity-70' : '',
    style.Italic ? 'italic' : '',
    style.Underline ? 'underline' : '',
  ]
    .filter((part) => part !== '')
    .join(' ');

const BASIC_COLOR_COUNT = 16;

const extendedColor = (codes: readonly number[], at: number): { Length: number; Fg: TAnsiColor | null } => {
  const mode = codes[at + 1];
  if (mode === 5) {
    const index = codes[at + 2];
    const basic = index !== undefined && index >= 0 && index < BASIC_COLOR_COUNT;
    return { Length: 2, Fg: basic ? (COLORS[index % COLORS.length] ?? null) : null };
  }
  if (mode === 2) return { Length: 4, Fg: null };
  return { Length: 0, Fg: null };
};

const applyCode = (style: IAnsiStyle, code: number): IAnsiStyle => {
  if (code === 0) return PLAIN_STYLE;
  if (code === 1) return { ...style, Bold: true };
  if (code === 2) return { ...style, Dim: true };
  if (code === 3) return { ...style, Italic: true };
  if (code === 4) return { ...style, Underline: true };
  if (code === 22) return { ...style, Bold: false, Dim: false };
  if (code === 23) return { ...style, Italic: false };
  if (code === 24) return { ...style, Underline: false };
  if (code === 39) return { ...style, Fg: null };
  if (code >= 30 && code <= 37) return { ...style, Fg: COLORS[code - 30] ?? null };
  if (code >= 90 && code <= 97) return { ...style, Fg: COLORS[code - 90] ?? null };
  return style;
};

const codeOf = (part: string): number => {
  if (part === '') return 0;
  if (part.includes(':')) return -1;
  return Number(part);
};

export const applySgr = (style: IAnsiStyle, params: string): IAnsiStyle => {
  const codes = params === '' ? [0] : params.split(';').map(codeOf);
  let next = style;
  for (let at = 0; at < codes.length; at += 1) {
    const code = codes[at] ?? 0;
    if (code === 38 || code === 48) {
      const color = extendedColor(codes, at);
      if (code === 38) next = { ...next, Fg: color.Fg };
      at += color.Length;
      continue;
    }
    next = applyCode(next, code);
  }
  return next;
};

const pushText = (segments: IAnsiSegment[], text: string, className: string) => {
  if (text === '') return;
  const previous = segments.at(-1);
  if (previous?.ClassName === className) previous.Text += text;
  else segments.push({ Text: text, ClassName: className });
};

const pushVisible = (segments: IAnsiSegment[], piece: string, className: string) => {
  let last = 0;
  for (const match of piece.matchAll(BIDI_CONTROLS)) {
    pushText(segments, piece.slice(last, match.index), className);
    segments.push({ Text: bidiLabel(match[0]), ClassName: BIDI_CLASS });
    last = match.index + match[0].length;
  }
  pushText(segments, piece.slice(last), className);
};

export const parseAnsi = (text: string, start: IAnsiStyle = PLAIN_STYLE): IAnsiResult => {
  const segments: IAnsiSegment[] = [];
  let style = start;
  let last = 0;
  const push = (until: number) => {
    if (until > last) pushVisible(segments, text.slice(last, until), classOf(style));
  };
  for (const match of text.matchAll(SEQUENCE)) {
    push(match.index);
    last = match.index + match[0].length;
    const params = match[1];
    if (match[3] === 'm' && match[2] === '' && params !== undefined && SGR_PARAMS.test(params)) style = applySgr(style, params);
  }
  push(text.length);
  return { Segments: segments, Style: style };
};

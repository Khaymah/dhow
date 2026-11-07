export const LINE_PARAM = 'line';
export const ATTEMPT_PARAM = 'attempt';

export const parsePositive = (value: string | null): number | null => {
  if (value === null || !/^[1-9][0-9]{0,9}$/.test(value)) return null;
  const number = Number(value);
  return Number.isSafeInteger(number) ? number : null;
};

export const readLine = (search: string): number | null => parsePositive(new URLSearchParams(search).get(LINE_PARAM));

export const readAttempt = (search: string): number | null => parsePositive(new URLSearchParams(search).get(ATTEMPT_PARAM));

export const lineUrl = (href: string, line: number, attempt: number): string => {
  const url = new URL(href);
  url.searchParams.set(ATTEMPT_PARAM, String(attempt));
  url.searchParams.set(LINE_PARAM, String(line));
  url.hash = '';
  return url.toString();
};

export const writeLine = (line: number | null, attempt: number): void => {
  const url = new URL(window.location.href);
  if (line === null) url.searchParams.delete(LINE_PARAM);
  else {
    url.searchParams.set(LINE_PARAM, String(line));
    url.searchParams.set(ATTEMPT_PARAM, String(attempt));
  }
  window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
};

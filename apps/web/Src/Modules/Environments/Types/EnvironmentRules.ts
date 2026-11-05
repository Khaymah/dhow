export const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/;
export const DESCRIPTION_MAX = 500;
export const REASON_MAX = 500;
export const PATTERN_MAX = 256;
export const PLUGIN_MAX = 256;
export const MAX_REFS = 50;
export const MAX_PLUGINS = 50;
export const MAX_REVIEWERS = 20;
export const WAIT_MAX = 43200;
export const TIMEOUT_MIN = 1;
export const TIMEOUT_MAX = 720;
export const DEFAULT_TIMEOUT_HOURS = 72;

const INVISIBLE = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u;
const FORBIDDEN_PART = /\.\.|\/\/|[\s~^:[\]{}]/;
const FULL_NAME_PREFIX = /^(refs|heads|tags)\//;

export type TPatternProblem = 'empty' | 'long' | 'invalid';

export const hasInvisible = (text: string): boolean => INVISIBLE.test(text);

export const patternProblem = (pattern: string): TPatternProblem | null => {
  if (pattern.trim() === '') return 'empty';
  if (pattern.length > PATTERN_MAX) return 'long';
  if (pattern.startsWith('/') || pattern.endsWith('/')) return 'invalid';
  if (FORBIDDEN_PART.test(pattern) || hasInvisible(pattern) || FULL_NAME_PREFIX.test(pattern)) return 'invalid';
  return null;
};

export const isWholeNumber = (text: string): boolean => /^\d{1,6}$/.test(text.trim());

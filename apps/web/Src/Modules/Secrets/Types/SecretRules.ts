import { byteLength } from '@/Core/Forms/ByteLength';

export const NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
export const RESERVED_PREFIX = 'DHOW_';
export const SECRET_MIN_BYTES = 8;
export const SECRET_MAX_BYTES = 32768;
export const VARIABLE_MAX_BYTES = 4096;
export const DESCRIPTION_MAX = 256;
export const USERNAME_MAX = 256;
export const HOST_MAX = 259;

export { byteLength };

export type TNameProblem = 'empty' | 'pattern' | 'reserved';

export const nameProblem = (name: string): TNameProblem | null => {
  if (name === '') return 'empty';
  if (!NAME_PATTERN.test(name)) return 'pattern';
  if (name.toUpperCase().startsWith(RESERVED_PREFIX)) return 'reserved';
  return null;
};

export type TValueProblem = 'short' | 'large' | 'nul';

export const secretValueProblem = (value: string): TValueProblem | null => {
  if (value.includes('\u0000')) return 'nul';
  const size = byteLength(value);
  if (size < SECRET_MIN_BYTES) return 'short';
  return size > SECRET_MAX_BYTES ? 'large' : null;
};

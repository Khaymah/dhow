import { z } from '@/Core/Forms/Zod';
import type { TManualInput } from '@/Modules/Projects/Api/RunsApi';

export type TInputValue = string | boolean;
export type TInputValues = Record<string, TInputValue>;

const REF_MAX = 255;
const VALUE_MAX = 4096;
const SHA = /^([0-9a-f]{40}|[0-9a-f]{64})$/;

const inputSchema = (input: TManualInput) => {
  if (input.Type === 'boolean') return z.boolean();
  if (input.Type === 'choice') {
    const choice = z.string().refine((value) => input.Options.includes(value), { error: 'runs.trigger.error.choice' });
    return input.Required ? choice : z.union([z.literal(''), choice]);
  }
  const text = z.string().max(VALUE_MAX, { error: 'runs.trigger.error.tooLong' });
  return input.Required ? text.trim().min(1, { error: 'runs.trigger.error.required' }) : text;
};

export const triggerSchema = (inputs: readonly TManualInput[]) =>
  z.object({
    Ref: z.string().trim().min(1, { error: 'runs.trigger.error.ref' }).max(REF_MAX, { error: 'runs.trigger.error.ref' }),
    Sha: z.union([z.literal(''), z.string().trim().regex(SHA, { error: 'runs.trigger.error.sha' })]),
    Inputs: z.object(Object.fromEntries(inputs.map((input) => [input.Name, inputSchema(input)]))),
  });

const initialValue = (input: TManualInput): TInputValue => {
  if (input.Type === 'boolean') return input.Default === 'true';
  if (input.Type === 'choice') return input.Default ?? (input.Required ? (input.Options[0] ?? '') : '');
  return input.Default ?? '';
};

export const initialValues = (inputs: readonly TManualInput[]): TInputValues => {
  const values: TInputValues = {};
  for (const input of inputs) values[input.Name] = initialValue(input);
  return values;
};

export const requestInputs = (inputs: readonly TManualInput[], values: TInputValues): Record<string, TInputValue> =>
  Object.fromEntries(
    inputs
      .map((input): [string, TInputValue | undefined] => [input.Name, values[input.Name]])
      .filter((entry): entry is [string, TInputValue] => entry[1] !== undefined && entry[1] !== ''),
  );

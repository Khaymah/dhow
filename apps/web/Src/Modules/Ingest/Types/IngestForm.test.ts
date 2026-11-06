import { describe, expect, it } from 'vitest';

import { ApiError } from '@/Core/Api/ApiError';
import {
  ACTION_SNIPPET,
  CLI_SNIPPET,
  daysOf,
  draftProblem,
  EMPTY_TOKEN_DRAFT,
  patternsOf,
  protectedReachOf,
  type ITokenDraft,
} from '@/Modules/Ingest/Types/IngestForm';
import { canManageIngest, canReadIngested } from '@/Modules/Ingest/Types/IngestAccess';
import { provenanceEntries, shortCommit, shortDigest, shortRefName } from '@/Modules/Ingest/Types/IngestDisplay';

const draft = (over: Partial<ITokenDraft> = {}): ITokenDraft => ({
  Name: 'ci',
  Patterns: [{ Kind: 'branch', Pattern: 'main' }],
  Days: '90',
  ...over,
});

describe('the token draft', () => {
  it('starts with 90 days and one empty branch pattern', () => {
    expect(EMPTY_TOKEN_DRAFT.Days).toBe('90');
    expect(EMPTY_TOKEN_DRAFT.Patterns).toEqual([{ Kind: 'branch', Pattern: '' }]);
    expect(draftProblem(EMPTY_TOKEN_DRAFT)).toBe('ingest.form.error.name');
  });

  it.each([
    ['1', 1],
    ['90', 90],
    [' 365 ', 365],
    ['0', null],
    ['366', null],
    ['', null],
    ['1.5', null],
    ['-3', null],
    ['9999', null],
    ['1e2', null],
  ])('reads %j days as %j', (text, expected) => {
    expect(daysOf(text)).toBe(expected);
  });

  it('trims the patterns, drops the empty and the repeated ones and keeps the kind', () => {
    expect(
      patternsOf(
        draft({
          Patterns: [
            { Kind: 'branch', Pattern: ' main ' },
            { Kind: 'branch', Pattern: 'main' },
            { Kind: 'tag', Pattern: 'main' },
            { Kind: 'tag', Pattern: '   ' },
          ],
        }),
      ),
    ).toEqual([
      { Kind: 'branch', Pattern: 'main' },
      { Kind: 'tag', Pattern: 'main' },
    ]);
  });

  it.each([
    ['a good draft', draft(), null],
    ['no name', draft({ Name: '  ' }), 'ingest.form.error.name'],
    ['a name over 64 characters', draft({ Name: 'a'.repeat(65) }), 'ingest.form.error.name'],
    ['a name of 64 characters', draft({ Name: 'a'.repeat(64) }), null],
    ['no pattern', draft({ Patterns: [{ Kind: 'branch', Pattern: ' ' }] }), 'ingest.form.error.patterns'],
    ['21 patterns', draft({ Patterns: Array.from({ length: 21 }, (_, index) => ({ Kind: 'branch' as const, Pattern: `b${String(index)}` })) }), 'ingest.form.error.patterns'],
    ['a pattern with a space inside', draft({ Patterns: [{ Kind: 'branch', Pattern: 'a b' }] }), 'ingest.form.error.pattern'],
    ['a pattern over 200 characters', draft({ Patterns: [{ Kind: 'branch', Pattern: 'a'.repeat(201) }] }), 'ingest.form.error.pattern'],
    ['400 days', draft({ Days: '400' }), 'ingest.form.error.days'],
  ])('judges %s', (_label, value, expected) => {
    expect(draftProblem(value)).toBe(expected);
  });
});

describe('the protected reach of an error', () => {
  const failure = (code: string, details: { Field: string; Message: string }[]) => new ApiError({ Code: code, Message: 'x', Details: details }, 422);

  it('lists the environments the server named', () => {
    expect(
      protectedReachOf(
        failure('INGEST_TOKEN_REACH_UNCONFIRMED', [
          { Field: 'AcknowledgeProtectedReach', Message: 'x' },
          { Field: 'ProtectedReach', Message: 'production' },
          { Field: 'ProtectedReach', Message: 'staging' },
        ]),
      ),
    ).toEqual(['production', 'staging']);
  });

  it('answers an empty list when the server named none, and null for any other error', () => {
    expect(protectedReachOf(failure('INGEST_TOKEN_REACH_UNCONFIRMED', []))).toEqual([]);
    expect(protectedReachOf(failure('VALIDATION_FAILED', [{ Field: 'ProtectedReach', Message: 'production' }]))).toBeNull();
    expect(protectedReachOf(new Error('x'))).toBeNull();
    expect(protectedReachOf(null)).toBeNull();
  });
});

describe('the snippets', () => {
  it('hold placeholders only and pass the Action output through env', () => {
    for (const snippet of [ACTION_SNIPPET, CLI_SNIPPET]) {
      expect(snippet).not.toMatch(/dhow_ing_/);
    }
    expect(ACTION_SNIPPET).toContain('${{ secrets.DHOW_INGEST_TOKEN }}');
    expect(ACTION_SNIPPET).toMatch(/- env:\n {4}DIGEST: \$\{\{ steps\.dhow\.outputs\.digest \}\}\n {2}run: echo "Dhow stored sha256:\$DIGEST"/);
    expect(ACTION_SNIPPET).not.toMatch(/run:.*\$\{\{/);
    expect(CLI_SNIPPET).toContain('<your-ingest-token>');
    expect(CLI_SNIPPET).not.toContain('--token ');
  });
});

describe('the roles', () => {
  it.each([
    ['owner', true, true],
    ['admin', true, true],
    ['maintainer', true, true],
    ['developer', false, true],
    ['viewer', false, false],
    [undefined, false, false],
  ] as const)('%s manages: %s, reads: %s', (role, manages, reads) => {
    expect(canManageIngest(role)).toBe(manages);
    expect(canReadIngested(role)).toBe(reads);
  });
});

describe('the display helpers', () => {
  it('shortens digests, commits and refs', () => {
    expect(shortDigest('0123456789abcdef0123')).toBe('0123456789ab');
    expect(shortCommit('0123456789abcdef')).toBe('01234567');
    expect(shortRefName('refs/heads/release/1.2')).toBe('release/1.2');
    expect(shortRefName('refs/tags/v1.0.0')).toBe('v1.0.0');
    expect(shortRefName('main')).toBe('main');
  });

  it('sorts the provenance by key', () => {
    expect(provenanceEntries({ Provider: 'x', Actor: 'y' })).toEqual([
      ['Actor', 'y'],
      ['Provider', 'x'],
    ]);
  });
});

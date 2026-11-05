import { describe, expect, it } from 'vitest';

import { environmentFixture, rulesFixture } from '@/Core/Testing/EnvironmentsBackend';
import type { TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { patternProblem } from '@/Modules/Environments/Types/EnvironmentRules';
import { draftOf, draftProblem, EMPTY_DRAFT, hasDraftChanges, pluginList, rulesInputOf } from '@/Modules/Environments/Types/RulesDraft';

const environment = environmentFixture() as unknown as TEnvironment;

describe('patternProblem', () => {
  it.each(['main', 'release/**', 'feature/*', 'v1.?', 'a_b-c'])('accepts %s', (pattern) => {
    expect(patternProblem(pattern)).toBeNull();
  });

  it.each([
    ['', 'empty'],
    ['   ', 'empty'],
    ['a'.repeat(257), 'long'],
    ['refs/heads/main', 'invalid'],
    ['heads/main', 'invalid'],
    ['tags/v1', 'invalid'],
    ['/main', 'invalid'],
    ['main/', 'invalid'],
    ['a//b', 'invalid'],
    ['a..b', 'invalid'],
    ['a b', 'invalid'],
    ['a~b', 'invalid'],
    ['a^b', 'invalid'],
    ['a:b', 'invalid'],
    ['a[b]', 'invalid'],
    ['a{b}', 'invalid'],
    ['a‮b', 'invalid'],
    ['a​b', 'invalid'],
  ])('refuses %j as %s', (pattern, problem) => {
    expect(patternProblem(pattern)).toBe(problem);
  });
});

describe('the rules draft', () => {
  it('reads the draft from an environment and sees no change in it', () => {
    const draft = draftOf(environment);
    expect(draft.WaitMinutes).toBe('10');
    expect(draft.UserIds).toHaveLength(1);
    expect(hasDraftChanges(draft, draftOf(environment), true)).toBe(false);
    expect(rulesInputOf(draft, draftOf(environment), false, true)).toEqual({});
  });

  it('puts only the changed rules in the input', () => {
    const original = draftOf(environment);
    const draft = { ...original, WaitMinutes: '15', AllowAdminBypass: true, Plugins: 'ghcr.io/acme\n\n  ghcr.io/dhow  ' };
    expect(rulesInputOf(draft, original, false, true)).toEqual({
      WaitMinutes: 15,
      AllowAdminBypass: true,
      AllowedPlugins: ['ghcr.io/acme', 'ghcr.io/dhow'],
    });
  });

  it('leaves the reviewers out when they are not shown, and compares them as sets', () => {
    const original = draftOf(environment);
    const reordered = { ...original, UserIds: [...original.UserIds].reverse() };
    expect(rulesInputOf(reordered, original, false, true)).toEqual({});
    const changed = { ...original, UserIds: [], TeamIds: ['0198a000-0000-7000-8000-0000000000d1'] };
    expect(rulesInputOf(changed, original, false, false)).toEqual({});
    expect(hasDraftChanges(changed, original, false)).toBe(false);
    expect(rulesInputOf(changed, original, false, true)).toEqual({
      Reviewers: { UserIds: [], TeamIds: ['0198a000-0000-7000-8000-0000000000d1'] },
    });
  });

  it('sends the refs on create only when they were edited', () => {
    expect(rulesInputOf(EMPTY_DRAFT, EMPTY_DRAFT, true, true)).toEqual({});
    expect(rulesInputOf({ ...EMPTY_DRAFT, RefsEdited: true }, EMPTY_DRAFT, true, true)).toEqual({ AllowedRefs: [] });
    const refs = [{ Kind: 'tag' as const, Pattern: ' v* ' }];
    expect(rulesInputOf({ ...EMPTY_DRAFT, Refs: refs, RefsEdited: true }, EMPTY_DRAFT, true, true)).toEqual({
      AllowedRefs: [{ Kind: 'tag', Pattern: 'v*' }],
    });
  });

  it('finds the first problem', () => {
    expect(draftProblem(EMPTY_DRAFT, true)).toBeNull();
    expect(draftProblem({ ...EMPTY_DRAFT, WaitMinutes: '-1' }, true)).toBe('environments.error.wait');
    expect(draftProblem({ ...EMPTY_DRAFT, WaitMinutes: '43201' }, true)).toBe('environments.error.wait');
    expect(draftProblem({ ...EMPTY_DRAFT, ApprovalTimeoutHours: '0' }, true)).toBe('environments.error.timeout');
    expect(draftProblem({ ...EMPTY_DRAFT, ApprovalTimeoutHours: '721' }, true)).toBe('environments.error.timeout');
    expect(draftProblem({ ...EMPTY_DRAFT, Description: 'x'.repeat(501) }, true)).toBe('environments.error.description');
    expect(draftProblem({ ...EMPTY_DRAFT, Refs: [{ Kind: 'branch', Pattern: 'a b' }] }, true)).toBe('environments.error.pattern.invalid');
    expect(draftProblem({ ...EMPTY_DRAFT, UserIds: Array.from({ length: 21 }, (_, index) => String(index)) }, true)).toBe(
      'environments.error.reviewers.many',
    );
    expect(draftProblem({ ...EMPTY_DRAFT, UserIds: Array.from({ length: 21 }, (_, index) => String(index)) }, false)).toBeNull();
    expect(draftProblem({ ...EMPTY_DRAFT, Plugins: 'a​b' }, true)).toBe('environments.error.plugins.invalid');
  });

  it('splits plugins by line', () => {
    expect(pluginList('a\n b \n\n')).toEqual(['a', 'b']);
    expect(draftOf({ ...environment, Rules: rulesFixture({ AllowedPlugins: ['x', 'y'] }) } as unknown as TEnvironment).Plugins).toBe('x\ny');
  });
});

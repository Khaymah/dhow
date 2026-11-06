import { describe, expect, it } from 'vitest';

import { targetFixture } from '@/Core/Testing/TargetsBackend';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import {
  createInputOf,
  defaultKeySecretName,
  draftOfTarget,
  draftProblem,
  EMPTY_TARGET_DRAFT,
  settingsOf,
  updateInputOf,
  type ITargetDraft,
} from '@/Modules/Environments/Types/TargetDraft';

const valid = (over: Partial<ITargetDraft> = {}): ITargetDraft => ({
  ...EMPTY_TARGET_DRAFT,
  Name: 'web',
  DeployPath: '/srv/web',
  Hosts: [{ Host: 'web-1.example.com', Port: '22' }],
  ...over,
});

describe('the problems of a target form', () => {
  it('accepts a complete form', () => {
    expect(draftProblem(valid(), true)).toBeNull();
  });

  it.each([
    ['a name with a capital', { Name: 'Web' }, 'targets.form.error.name'],
    ['a name that starts with a dash', { Name: '-web' }, 'targets.form.error.name'],
    ['no user', { Username: ' ' }, 'targets.form.error.username'],
    ['root without the option', { Username: 'root' }, 'targets.form.error.root'],
    ['a relative path', { DeployPath: 'srv/web' }, 'targets.form.error.path'],
    ['a path with a space', { DeployPath: '/srv/my app' }, 'targets.form.error.path'],
    ['a bad secret name', { KeySecretName: '1key' }, 'targets.form.error.keySecret'],
    ['no server', { Hosts: [] }, 'targets.form.error.noHost'],
    ['an empty address', { Hosts: [{ Host: ' ', Port: '22' }] }, 'targets.form.error.host'],
    ['an address with a space', { Hosts: [{ Host: 'a b', Port: '22' }] }, 'targets.form.error.host'],
    ['a port of zero', { Hosts: [{ Host: 'a.example.com', Port: '0' }] }, 'targets.form.error.port'],
    ['a port too large', { Hosts: [{ Host: 'a.example.com', Port: '65536' }] }, 'targets.form.error.port'],
    ['one kept release', { KeepReleases: '1' }, 'targets.form.error.keep'],
    ['too many kept releases', { KeepReleases: '51' }, 'targets.form.error.keep'],
    ['a shared name with a slash', { Shared: '.env\nconfig/db' }, 'targets.form.error.shared'],
    ['a bad compose project', { Kind: 'compose', ProjectName: 'Stack' }, 'targets.form.error.projectName'],
  ] satisfies [string, Partial<ITargetDraft>, string][])('refuses %s', (_, over, key) => {
    expect(draftProblem(valid(over), true)).toBe(key);
  });

  it('lets root through only when the option is on', () => {
    expect(draftProblem(valid({ Username: 'root', AllowRoot: true }), true)).toBeNull();
  });

  it('checks the batches only for a group of servers', () => {
    const two = [
      { Host: 'a.example.com', Port: '22' },
      { Host: 'b.example.com', Port: '22' },
    ];
    expect(draftProblem(valid({ BatchSize: '9' }), true)).toBeNull();
    expect(draftProblem(valid({ Hosts: two, BatchSize: '3' }), true)).toBe('targets.form.error.batch');
    expect(draftProblem(valid({ Hosts: two, BatchSize: '0' }), true)).toBe('targets.form.error.batch');
    expect(draftProblem(valid({ Hosts: two, BatchPauseSeconds: '601' }), true)).toBe('targets.form.error.pause');
    expect(draftProblem(valid({ Hosts: two, BatchSize: '2', BatchPauseSeconds: '600' }), true)).toBeNull();
  });

  it('does not ask for a name when it changes an existing target', () => {
    expect(draftProblem(valid({ Name: '' }), false)).toBeNull();
  });
});

describe('the request a form builds', () => {
  it('sends only the block of the chosen kind and drops the empty fields', () => {
    const input = createInputOf(
      valid({
        ReloadCommand: ' sudo systemctl reload web ',
        Shared: '.env, storage\n\nlogs',
        ProjectName: 'ignored',
        AllowUnsafeCompose: true,
        AllowedImages: 'ghcr.io/acme/\n',
      }),
    );
    expect(input).toEqual({
      Name: 'web',
      Kind: 'ssh',
      Username: 'deploy',
      DeployPath: '/srv/web',
      KeepReleases: 5,
      Settings: { Ssh: { ReloadCommand: 'sudo systemctl reload web', Shared: ['.env', 'storage', 'logs'] } },
      AllowedImages: ['ghcr.io/acme/'],
      Hosts: [{ Host: 'web-1.example.com', Port: 22 }],
    });
  });

  it('sends the rolling settings for a group and the compose block for a compose target', () => {
    const two = [
      { Host: 'a.example.com', Port: '22' },
      { Host: 'b.example.com', Port: '2222' },
    ];
    const input = createInputOf(valid({ Kind: 'compose', Hosts: two, BatchSize: '2', BatchPauseSeconds: '30', ProjectName: 'stack', ComposeFile: 'compose.prod.yaml' }));
    expect(input.Settings).toEqual({
      Rolling: { BatchSize: 2, BatchPauseSeconds: 30 },
      Compose: { ProjectName: 'stack', ComposeFile: 'compose.prod.yaml' },
    });
    expect(input.Hosts).toEqual([
      { Host: 'a.example.com', Port: 22 },
      { Host: 'b.example.com', Port: 2222 },
    ]);
  });

  it('names the secret only when one was typed', () => {
    expect(createInputOf(valid())).not.toHaveProperty('KeySecretName');
    expect(createInputOf(valid({ KeySecretName: ' WEB_KEY ' })).KeySecretName).toBe('WEB_KEY');
  });

  it('turns the settings of an existing target into a form and back without losing anything', () => {
    const target = targetFixture({
      Settings: {
        Rolling: { BatchSize: 2, BatchPauseSeconds: 15 },
        AllowRoot: false,
        Ssh: { ReloadCommand: 'reload', PreSwitchCommand: 'pre', PostSwitchCommand: 'post', Shared: ['.env'] },
      },
      AllowedImages: ['ghcr.io/acme/'],
    }) as unknown as TTarget;
    const draft = draftOfTarget(target);
    expect(draft.BatchSize).toBe('2');
    expect(draft.Shared).toBe('.env');
    expect(draft.Hosts).toHaveLength(3);
    const update = updateInputOf(draft);
    expect(update.Settings).toEqual({
      Rolling: { BatchSize: 2, BatchPauseSeconds: 15 },
      Ssh: { ReloadCommand: 'reload', PreSwitchCommand: 'pre', PostSwitchCommand: 'post', Shared: ['.env'] },
    });
    expect(update.AllowedImages).toEqual(['ghcr.io/acme/']);
    expect(settingsOf({ ...draft, Kind: 'compose' })).toEqual({ Rolling: { BatchSize: 2, BatchPauseSeconds: 15 } });
  });

  it('proposes a secret name from the target name', () => {
    expect(defaultKeySecretName('web-app')).toBe('WEB_APP_SSH_KEY');
  });
});

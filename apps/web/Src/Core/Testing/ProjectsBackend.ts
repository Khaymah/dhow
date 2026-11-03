import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope, ORG_ID } from '@/Core/Testing/Fetch';

export type TFixture = Record<string, unknown> & { Id: string };

export const PROJECT_ID = '0198a000-0000-7000-8000-000000000301';
export const RUN_ID = '0198a000-0000-7000-8000-000000000401';
export const JOB_ID = '0198a000-0000-7000-8000-000000000501';
export const ARTIFACT_ID = '0198a000-0000-7000-8000-000000000601';
export const FINGERPRINT = 'SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ';
export const WEBHOOK_SECRET = 'whsec-super-secret-value-0123456789abcdefghij';
export const SHA = '0123456789abcdef0123456789abcdef01234567';

export const projectFixture = (overrides: Partial<TFixture> = {}): TFixture => ({
  Id: PROJECT_ID,
  OrganizationId: ORG_ID,
  Slug: 'web-app',
  Name: 'Web app',
  PipelinePath: '.dhow.yml',
  ForkPolicy: 'approval',
  State: 'active',
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  ...overrides,
});

export const repositoryFixture = (overrides: Partial<TFixture> = {}): TFixture => ({
  Id: '0198a000-0000-7000-8000-000000000701',
  ProjectId: PROJECT_ID,
  Provider: 'generic',
  CloneUrl: 'ssh://git@git.example.com/team/app.git',
  Transport: 'ssh',
  FullName: 'team/app',
  DefaultBranch: 'main',
  WebhookUrl: 'https://dhow.example.com/webhooks/git/0123456789abcdef0123456789abcdef',
  DeployKeyPublic: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKey dhow',
  PendingHostKeyFingerprint: FINGERPRINT,
  State: 'host_key_unconfirmed',
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  ...overrides,
});

export const runFixture = (overrides: Partial<TFixture> = {}): TFixture => ({
  Id: RUN_ID,
  ProjectId: PROJECT_ID,
  Number: 7,
  Trigger: 'push',
  Ref: 'refs/heads/main',
  Sha: SHA,
  IsFork: false,
  Actor: 'sara',
  State: 'succeeded',
  DefinitionPath: '.dhow.yml',
  StartedAt: '2026-11-01T08:00:00Z',
  FinishedAt: '2026-11-01T08:01:05Z',
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-01T08:01:05Z',
  ...overrides,
});

export const runJobFixture = (overrides: Partial<TFixture> = {}): TFixture => ({
  Id: '0198a000-0000-7000-8000-000000000801',
  Key: 'build',
  JobId: 'build',
  Name: 'build',
  Matrix: {},
  Needs: [],
  State: 'succeeded',
  CurrentJobId: JOB_ID,
  Attempts: 1,
  StartedAt: '2026-11-01T08:00:00Z',
  FinishedAt: '2026-11-01T08:00:30Z',
  ...overrides,
});

export const artifactFixture = (overrides: Partial<TFixture> = {}): TFixture => ({
  Id: ARTIFACT_ID,
  RunId: RUN_ID,
  Name: 'web-dist',
  RunJobKey: 'build',
  SizeBytes: 2_621_440,
  Digest: 'a'.repeat(64),
  Untrusted: false,
  CreatedAt: '2026-11-01T08:01:00Z',
  ExpiresAt: '2026-11-08T08:01:00Z',
  ...overrides,
});

export const detailFixture = (run: TFixture = runFixture(), jobs: TFixture[] = [runJobFixture()], issues: unknown[] = []) => ({
  Run: run,
  Jobs: jobs,
  Issues: issues,
  ImagePins: [],
});

export const page = (items: readonly unknown[]) =>
  envelope({ Success: true, Data: items, Meta: { Page: 1, Limit: 20, Total: items.length } });

export const ok = (data: unknown, status = 200) => envelope({ Success: true, Data: data }, status);

export const notLinked = () => backendFailure('REPOSITORY_NOT_LINKED', 'المشروع غير مربوط بمستودع.', 404);

export interface IProjectsBackend {
  projects?: TFixture[];
  project?: () => Response | Promise<Response>;
  repository?: () => Response | Promise<Response>;
  runs?: (url: URL) => Response | Promise<Response>;
  run?: () => Response | Promise<Response>;
  artifacts?: () => Response | Promise<Response>;
  inputs?: () => Response | Promise<Response>;
  write?: (request: Request, url: URL) => Response | Promise<Response> | undefined;
}

const PROJECT = /\/projects\/[0-9a-f-]{36}$/;
const RUN = /\/runs\/[0-9a-f-]{36}$/;

export const projectsRoute =
  (backend: IProjectsBackend): TRouteHandler =>
  (request, url) => {
    const path = url.pathname;
    if (request.method !== 'GET') return backend.write?.(request, url);
    if (path.endsWith('/projects')) return page(backend.projects ?? [projectFixture()]);
    if (PROJECT.test(path)) return backend.project?.() ?? ok(projectFixture({ CallerRole: 'owner' }));
    if (path.endsWith('/repository')) return backend.repository?.() ?? ok(repositoryFixture());
    if (path.endsWith('/manual-inputs')) {
      return backend.inputs?.() ?? ok({ Known: true, Enabled: true, Ref: 'refs/heads/main', Sha: SHA, Inputs: [] });
    }
    if (path.endsWith('/runs')) return backend.runs?.(url) ?? page([runFixture()]);
    if (path.endsWith('/artifacts')) return backend.artifacts?.() ?? ok([artifactFixture()]);
    if (RUN.test(path)) return backend.run?.() ?? ok(detailFixture());
    return undefined;
  };

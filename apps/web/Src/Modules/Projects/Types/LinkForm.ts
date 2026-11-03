import { z } from '@/Core/Forms/Zod';

export const CLONE_URL_MAX = 2048;
export const BRANCH_MAX = 244;

const ADDRESS = /^(?:(?:ssh|https):\/\/\S+|[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:\S+)$/;
const BRANCH = /^(?!-)(?!.*\.\.)(?!.*@\{)(?!.*\.lock$)(?!.*\/$)[\w./+=-]+$/;

export const linkFormSchema = z.object({
  CloneUrl: z
    .string()
    .trim()
    .min(1, { error: 'repository.error.url' })
    .max(CLONE_URL_MAX, { error: 'repository.error.url' })
    .regex(ADDRESS, { error: 'repository.error.url' }),
  DefaultBranch: z
    .string()
    .trim()
    .min(1, { error: 'repository.error.branch' })
    .max(BRANCH_MAX, { error: 'repository.error.branch' })
    .regex(BRANCH, { error: 'repository.error.branch' }),
});

export const EXAMPLES = {
  ssh: 'ssh://git@git.example.com/team/app.git',
  scp: 'git@git.example.com:team/app.git',
  branch: 'main',
  tag: 'refs/tags/v1',
  deployKeysMenu: 'Deploy keys',
  writeOption: 'write access',
  webhooksMenu: 'Webhooks',
  contentType: 'application/json',
  hostKeyCommand: 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub',
  manualKey: 'on.manual',
} as const;

export const FINGERPRINT = /^SHA256:[A-Za-z0-9+/]{43}$/;

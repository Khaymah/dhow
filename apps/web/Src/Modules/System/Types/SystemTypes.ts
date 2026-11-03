import type { components } from '@/Core/Api/Schema';
import type { TIconName } from '@/Core/Ui/Icons';

export type TMeta = components['schemas']['Meta'];
export type THealthStatus = components['schemas']['HealthStatus'];
export type THealthCheck = components['schemas']['HealthCheck'];
export type TCheckName = THealthCheck['Name'];

export const CHECK_NAMES = ['database', 'migrations', 'log_partitions', 'settings', 'keyring'] as const satisfies readonly TCheckName[];

export const CHECK_ICONS: Record<TCheckName, TIconName> = {
  database: 'server',
  migrations: 'layers',
  log_partitions: 'file',
  settings: 'settings',
  keyring: 'key',
};

export const isCheckName = (value: unknown): value is TCheckName => CHECK_NAMES.some((name) => name === value);

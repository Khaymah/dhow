import { useT } from '@/I18n/useT';
import type { TRun } from '@/Modules/Projects/Api/RunsApi';
import { isKnownReason } from '@/Modules/Projects/Types/Reasons';


const MINUTE = 60;
const HOUR = 3600;

export const secondsBetween = (from: string, to: string | number): number | null => {
  const start = Date.parse(from);
  const end = typeof to === 'number' ? to : Date.parse(to);
  if (Number.isNaN(start) || Number.isNaN(end)) return null;
  return Math.max(0, Math.floor((end - start) / 1000));
};

export const useRunText = () => {
  const t = useT();
  const duration = (from: string, to: string | number): string => {
    const seconds = secondsBetween(from, to);
    if (seconds === null) return '';
    if (seconds < MINUTE) return t('jobs.duration.seconds', { s: seconds });
    if (seconds < HOUR) return t('jobs.duration.minutes', { m: Math.floor(seconds / MINUTE), s: seconds % MINUTE });
    return t('jobs.duration.hours', { h: Math.floor(seconds / HOUR), m: Math.floor((seconds % HOUR) / MINUTE) });
  };
  return {
    reason: (code: string): string => (isKnownReason(code) ? t(`runs.reason.${code}`) : t('runs.reason.unknown', { code })),
    duration,
    runDuration: (run: Pick<TRun, 'StartedAt' | 'FinishedAt'>, now: number): string =>
      run.StartedAt === undefined ? '' : duration(run.StartedAt, run.FinishedAt ?? now),
    actor: (run: Pick<TRun, 'Actor' | 'Trigger' | 'TriggeredByUserId'>): string => {
      if (run.Actor !== undefined && run.Actor !== '') return run.Actor;
      if (run.TriggeredByUserId !== undefined) return t('runs.actor.member');
      return run.Trigger === 'api' ? t('runs.actor.token') : t('runs.actor.unknown');
    },
    size: (bytes: number): string => {
      const mib = 1024 ** 2;
      const kib = 1024;
      if (bytes >= mib) return t('jobs.size.mib', { n: (bytes / mib).toFixed(1) });
      if (bytes >= kib) return t('runs.size.kib', { n: Math.max(1, Math.round(bytes / kib)) });
      return t('jobs.size.bytes', { n: bytes });
    },
  };
};

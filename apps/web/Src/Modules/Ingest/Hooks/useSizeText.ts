import { useT } from '@/I18n/useT';

const KIB = 1024;
const MIB = KIB ** 2;
const GIB = KIB ** 3;

export const useSizeText = (): ((bytes: number) => string) => {
  const t = useT();
  return (bytes) => {
    if (bytes >= GIB) return t('jobs.size.gib', { n: (bytes / GIB).toFixed(1) });
    if (bytes >= MIB) return t('jobs.size.mib', { n: (bytes / MIB).toFixed(1) });
    if (bytes >= KIB) return t('runs.size.kib', { n: Math.max(1, Math.round(bytes / KIB)) });
    return t('jobs.size.bytes', { n: bytes });
  };
};

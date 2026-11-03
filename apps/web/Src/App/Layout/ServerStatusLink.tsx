import { useNavigate } from '@tanstack/react-router';

import { cx } from '@/Core/Ui/ClassNames';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import { useReadiness } from '@/Modules/System/Hooks/useReadiness';

type TServerState = 'ok' | 'down' | 'checking';

const DOT: Record<TServerState, string> = {
  ok: 'bg-accent-green',
  down: 'bg-accent-coral',
  checking: 'bg-track',
};

export const ServerStatusLink = () => {
  const t = useT();
  const navigate = useNavigate();
  const readiness = useReadiness();
  const state: TServerState = readiness.data?.Status === 'ok' ? 'ok' : readiness.isPending ? 'checking' : 'down';
  const label = t(`shell.status.${state}`);

  return (
    <a
      href="/system"
      data-state={state}
      aria-label={t('shell.status.open', { state: label })}
      onClick={followInApp(() => {
        void navigate({ to: '/system' });
      })}
      className={cx(
        'flex h-btn items-center gap-2 rounded-pill border border-line-2 px-3 text-label-sm whitespace-nowrap text-secondary no-underline',
        'transition-colors duration-140 hover:bg-hover hover:text-primary max-md:px-2.5',
      )}
    >
      <i aria-hidden="true" className={cx('size-2 flex-none rounded-full', DOT[state])} />
      <span className="max-md:hidden">{label}</span>
      <span aria-hidden="true" className="md:hidden">{t(`shell.status.short.${state}`)}</span>
    </a>
  );
};

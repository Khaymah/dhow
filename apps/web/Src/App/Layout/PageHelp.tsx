import { useRouterState } from '@tanstack/react-router';
import { useId } from 'react';

import { matchSection } from '@/App/Navigation/Sections';
import { docLinkFor } from '@/Core/Help/DocLinks';
import { docsUrl } from '@/Core/Help/DocsUrl';
import { Icon } from '@/Core/Ui/Icon';
import type { TMessageKey } from '@/I18n/Messages';
import { useI18n } from '@/I18n/useI18n';

export const PageHelp = () => {
  const { Locale, t } = useI18n();
  const hintId = useId();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const section = matchSection(pathname)?.section;
  const path = section ? docLinkFor(section.id) : null;
  const href = path === null ? null : docsUrl(path, Locale);
  if (!section || href === null) return null;
  const hint = t(`help.hint.${section.id}` satisfies TMessageKey);

  return (
    <>
      <a
        href={href}
        target="_blank"
        rel="noreferrer noopener"
        title={hint}
        aria-describedby={hintId}
        className="inline-flex h-btn-sm cursor-pointer items-center gap-2 rounded-btn border border-line-2 px-3 text-label-sm whitespace-nowrap text-primary transition-colors duration-140 ease-dhow hover:bg-surface-raised max-md:min-h-11"
      >
        <Icon name="help" size={15} />
        <span>{t('help.button')}</span>
        <span aria-hidden="true">{'↗'}</span>
        <span className="sr-only">{`(${t('api.error.newTab')})`}</span>
      </a>
      <span id={hintId} className="sr-only">
        {hint}
      </span>
    </>
  );
};

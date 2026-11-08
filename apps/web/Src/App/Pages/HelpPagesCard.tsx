import { SECTIONS } from '@/App/Navigation/Sections';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { docLinkFor } from '@/Core/Help/DocLinks';
import { docsUrl } from '@/Core/Help/DocsUrl';
import { Card } from '@/Core/Ui/Card';
import type { TMessageKey } from '@/I18n/Messages';
import { useI18n } from '@/I18n/useI18n';

export const HelpPagesCard = () => {
  const { Locale, t } = useI18n();
  const role = useCurrentMembership()?.Role;
  const owner = useMe().data?.InstanceOwner === true;
  const rows = SECTIONS.flatMap((section) => {
    const path = docLinkFor(section.id);
    const href = path === null ? null : docsUrl(path, Locale);
    return href !== null && section.allow(role, owner) ? [{ section, href }] : [];
  });

  return (
    <Card title={t('help.pages.title')} icon="layers">
      <ul className="m-0 grid list-none p-0">
        {rows.map(({ section, href }) => (
          <li key={section.id} className="grid gap-0.5 border-t border-line px-card py-3 first:border-t-0">
            <a
              href={href}
              target="_blank"
              rel="noreferrer noopener"
              aria-label={t('help.pages.open', { page: t(section.label) })}
              className="w-fit text-label text-primary underline underline-offset-4"
            >
              <bdi>{t(section.label)}</bdi>
              <span aria-hidden="true">{' ↗'}</span>
            </a>
            <span className="text-body-sm text-secondary">{t(`help.hint.${section.id}` satisfies TMessageKey)}</span>
          </li>
        ))}
      </ul>
    </Card>
  );
};

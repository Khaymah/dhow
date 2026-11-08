import { Card } from '@/Core/Ui/Card';
import { docsUrl, DOCS_BASE_URL } from '@/Core/Help/DocsUrl';
import { Icon } from '@/Core/Ui/Icon';
import { useI18n } from '@/I18n/useI18n';
import type { TMessageKey } from '@/I18n/Messages';

interface IAboutLink {
  label: TMessageKey;
  path: string;
}

const DOC_LINKS: readonly IAboutLink[] = [
  { label: 'about.link.install', path: 'install.md' },
  { label: 'about.link.doctor', path: 'guides/about-and-doctor.md' },
  { label: 'about.link.cli', path: 'cli.md' },
  { label: 'about.link.security', path: 'security-model.md' },
  { label: 'about.link.editions', path: 'editions-and-licensing.md' },
];

const LICENSE_FILE = 'LICENSE.md';

const ExternalLink = ({ href, label }: { href: string; label: string }) => {
  const { t } = useI18n();

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      <a
        href={href}
        target="_blank"
        rel="noreferrer noopener"
        className="flex min-h-btn items-center gap-3 px-card py-3 text-label text-primary no-underline hover:bg-surface-raised"
      >
        <Icon name="file" size={18} className="text-secondary" />
        <span className="flex-1">{label}</span>
        <span aria-hidden="true">{'↗'}</span>
        <span className="sr-only">{`(${t('api.error.newTab')})`}</span>
      </a>
    </li>
  );
};

export const AboutLinksCard = () => {
  const { Locale, t } = useI18n();

  return (
    <Card title={t('about.links.title')} icon="info">
      <ul aria-label={t('about.links.title')} className="m-0 grid list-none p-0">
        {DOC_LINKS.map((link) => {
          const href = docsUrl(link.path, Locale);
          return href === null ? null : <ExternalLink key={link.path} href={href} label={t(link.label)} />;
        })}
        <ExternalLink href={`${DOCS_BASE_URL}${LICENSE_FILE}`} label={t('about.link.license')} />
      </ul>
    </Card>
  );
};

const PAGES_BY_DOC: Readonly<Record<string, readonly string[]>> = {
  'guides/home.md': ['home'],
  'guides/projects.md': ['projects'],
  'guides/jobs.md': ['jobs'],
  'guides/environments.md': ['approvals'],
  'runner/run.md': ['runners'],
  'runner/register.md': ['runnerTokens'],
  'guides/organization.md': ['members', 'teams', 'invitations', 'apiKeys', 'orgSettings', 'profile', 'security', 'preferences', 'organizations'],
  'guides/secrets.md': ['secrets'],
  'security-model.md': ['audit'],
  'guides/about-and-doctor.md': ['system', 'about'],
  'guides/instance-settings.md': ['systemSettings'],
  'guides/help-and-faq.md': ['help'],
  'guides/notifications.md': ['notifications'],
};

export const DOC_LINKS: Readonly<Record<string, string>> = Object.fromEntries(
  Object.entries(PAGES_BY_DOC).flatMap(([doc, pages]) => pages.map((page) => [page, doc] as const)),
);

export const docLinkFor = (sectionId: string): string | null => DOC_LINKS[sectionId] ?? null;

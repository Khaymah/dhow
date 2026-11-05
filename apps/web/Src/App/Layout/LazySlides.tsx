import { Suspense } from 'react';

import { isThemeEditSlide, oidcOf, secretsTargetOf, type TSlideId } from '@/App/Panels';
import { lazyComponent, type IPreloadable, type TLazyComponent } from '@/Core/Ui/LazyComponent';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';

const ThemeEditorBody = lazyComponent(() =>
  import('@/Modules/Appearance/Components/ThemeEditorBody').then((module) => module.ThemeEditorBody),
);
const ThemeEditorFooter = lazyComponent(() =>
  import('@/Modules/Appearance/Components/ThemeEditorFooter').then((module) => module.ThemeEditorFooter),
);
const PasswordBody = lazyComponent(() =>
  import('@/Modules/Session/Components/PasswordBody').then((module) => module.PasswordBody),
);
const PasswordFooter = lazyComponent(() =>
  import('@/Modules/Session/Components/PasswordFooter').then((module) => module.PasswordFooter),
);
const TotpBody = lazyComponent(() => import('@/Modules/Mfa/Components/TotpBody').then((module) => module.TotpBody));
const TotpFooter = lazyComponent(() => import('@/Modules/Mfa/Components/TotpFooter').then((module) => module.TotpFooter));
const TokenNewBody = lazyComponent(() =>
  import('@/Modules/Tokens/Components/TokenNewBody').then((module) => module.TokenNewBody),
);
const OrganizationTokenNewBody = lazyComponent(() =>
  import('@/Modules/Tokens/Components/OrganizationTokensBody').then((module) => module.OrganizationTokenNewBody),
);
const OrganizationNewBody = lazyComponent(() =>
  import('@/Modules/Organization/Components/OrganizationNewBody').then((module) => module.OrganizationNewBody),
);
const InviteBody = lazyComponent(() => import('@/Modules/Organization/Components/InviteBody').then((module) => module.InviteBody));
const InviteFooter = lazyComponent(() =>
  import('@/Modules/Organization/Components/InviteFooter').then((module) => module.InviteFooter),
);
const TransferBody = lazyComponent(() =>
  import('@/Modules/Organization/Components/TransferBody').then((module) => module.TransferBody),
);
const TeamNewBody = lazyComponent(() => import('@/Modules/Organization/Components/TeamNewBody').then((module) => module.TeamNewBody));
const MemberRemoveBody = lazyComponent(() =>
  import('@/Modules/Organization/Components/MemberRemoveBody').then((module) => module.MemberRemoveBody),
);
const LeaveBody = lazyComponent(() => import('@/Modules/Organization/Components/LeaveBody').then((module) => module.LeaveBody));
const OidcProviderBody = lazyComponent(() =>
  import('@/Modules/Settings/Components/OidcProviderBody').then((module) => module.OidcProviderBody),
);
const ProjectNewBody = lazyComponent(() =>
  import('@/Modules/Projects/Components/ProjectNewBody').then((module) => module.ProjectNewBody),
);
const RepositoryLinkBody = lazyComponent(() =>
  import('@/Modules/Projects/Components/RepositoryLinkBody').then((module) => module.RepositoryLinkBody),
);
const TriggerRunBody = lazyComponent(() =>
  import('@/Modules/Projects/Components/TriggerRunBody').then((module) => module.TriggerRunBody),
);
const RunnerNewBody = lazyComponent(() => import('@/Modules/Runners/Components/RunnerNewBody').then((module) => module.RunnerNewBody));

const SecretFormBody = lazyComponent(() =>
  import('@/Modules/Secrets/Components/SecretFormBody').then((module) => module.SecretFormBody),
);
const VariableFormBody = lazyComponent(() =>
  import('@/Modules/Secrets/Components/VariableFormBody').then((module) => module.VariableFormBody),
);
const RegistryFormBody = lazyComponent(() =>
  import('@/Modules/Secrets/Components/RegistryFormBody').then((module) => module.RegistryFormBody),
);

type TSlidePart = Exclude<
  TSlideId,
  `theme-edit-${string}` | `oidc-edit-${string}` | `secret-edit-${string}` | `variable-edit-${string}` | `registry-edit-${string}`
>;

const PARTS: Record<TSlidePart, readonly IPreloadable[]> = {
  'theme-edit': [ThemeEditorBody, ThemeEditorFooter],
  'account-password': [PasswordBody, PasswordFooter],
  'account-totp': [TotpBody, TotpFooter],
  'account-token-new': [TokenNewBody],
  'organization-token-new': [OrganizationTokenNewBody],
  'organization-new': [OrganizationNewBody],
  'organization-invite': [InviteBody, InviteFooter],
  'organization-transfer': [TransferBody],
  'team-new': [TeamNewBody],
  'runner-new': [RunnerNewBody],
  'member-remove': [MemberRemoveBody],
  'organization-leave': [LeaveBody],
  'oidc-new': [OidcProviderBody],
  'project-new': [ProjectNewBody],
  'repository-link': [RepositoryLinkBody],
  'run-trigger': [TriggerRunBody],
  'secret-new': [SecretFormBody],
  'variable-new': [VariableFormBody],
  'registry-new': [RegistryFormBody],
};

const partOf = (id: TSlideId): TSlidePart => {
  if (isThemeEditSlide(id)) return 'theme-edit';
  if (oidcOf(id) !== null) return 'oidc-new';
  const target = secretsTargetOf(id);
  if (target !== null) return `${target.Kind}-new` as TSlidePart;
  return id as TSlidePart;
};

const preloadAll = (components: readonly IPreloadable[]): Promise<void> =>
  Promise.all(components.map((component) => component.preload())).then(() => undefined);

export const preloadSlide = (id: TSlideId): Promise<void> => preloadAll(PARTS[partOf(id)]);

export const preloadAllSlides = (): Promise<void> => preloadAll(Object.values(PARTS).flat());

const Loading = () => {
  const t = useT();
  return <Skeleton label={t('ui.loading')} lines={5} />;
};

const withSkeleton = <P extends object>(Component: TLazyComponent<P>) => {
  const Body = (props: P) => (
    <Suspense fallback={<Loading />}>
      <Component {...props} />
    </Suspense>
  );
  return Body;
};

const withNothing = <P extends object>(Component: TLazyComponent<P>) => {
  const Part = (props: P) => (
    <Suspense fallback={null}>
      <Component {...props} />
    </Suspense>
  );
  return Part;
};

export const LazyThemeEditorBody = withSkeleton(ThemeEditorBody);
export const LazyThemeEditorFooter = withNothing(ThemeEditorFooter);
export const LazyPasswordBody = withSkeleton(PasswordBody);
export const LazyPasswordFooter = withNothing(PasswordFooter);
export const LazyTotpBody = withSkeleton(TotpBody);
export const LazyTotpFooter = withNothing(TotpFooter);
export const LazyTokenNewBody = withSkeleton(TokenNewBody);
export const LazyOrganizationTokenNewBody = withSkeleton(OrganizationTokenNewBody);
export const LazyOrganizationNewBody = withSkeleton(OrganizationNewBody);
export const LazyInviteBody = withSkeleton(InviteBody);
export const LazyInviteFooter = withNothing(InviteFooter);
export const LazyTransferBody = withSkeleton(TransferBody);
export const LazyTeamNewBody = withSkeleton(TeamNewBody);
export const LazyRunnerNewBody = withSkeleton(RunnerNewBody);
export const LazyOidcProviderBody = withSkeleton(OidcProviderBody);
export const LazyMemberRemoveBody = withSkeleton(MemberRemoveBody);
export const LazyLeaveBody = withSkeleton(LeaveBody);
export const LazyProjectNewBody = withSkeleton(ProjectNewBody);
export const LazyRepositoryLinkBody = withSkeleton(RepositoryLinkBody);
export const LazyTriggerRunBody = withSkeleton(TriggerRunBody);
export const LazySecretFormBody = withSkeleton(SecretFormBody);
export const LazyVariableFormBody = withSkeleton(VariableFormBody);
export const LazyRegistryFormBody = withSkeleton(RegistryFormBody);

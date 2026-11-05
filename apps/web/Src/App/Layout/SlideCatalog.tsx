import {
  LazyInviteBody,
  LazyInviteFooter,
  LazyLeaveBody,
  LazyMemberRemoveBody,
  LazyOidcProviderBody,
  LazyOrganizationNewBody,
  LazyOrganizationTokenNewBody,
  LazyPasswordBody,
  LazyPasswordFooter,
  LazyProjectNewBody,
  LazyRegistryFormBody,
  LazyRepositoryLinkBody,
  LazyRunnerNewBody,
  LazySecretFormBody,
  LazyTeamNewBody,
  LazyThemeEditorBody,
  LazyThemeEditorFooter,
  LazyTokenNewBody,
  LazyTotpBody,
  LazyTotpFooter,
  LazyTransferBody,
  LazyTriggerRunBody,
  LazyVariableFormBody,
} from '@/App/Layout/LazySlides';
import { oidcOf, secretsTargetOf, themeOf, type ISecretsEditTarget, type TSlideId, type TStaticSlideId } from '@/App/Panels';
import type { ICustomTheme } from '@/Core/Theme/ThemeTokens';
import type { ISlide } from '@/Core/Ui/SlideStack';
import type { TTranslate } from '@/I18n/I18nContext';

export interface ISlideContext {
  t: TTranslate;
  close: () => void;
  openTeam: (teamId: string) => void;
  openOrganization: () => void;
  memberId: string | undefined;
  openMembers: () => void;
  openHome: () => void;
  draftName: string | undefined;
  customThemes: readonly ICustomTheme[];
  projectId: string | undefined;
  openProject: (projectId: string) => void;
  openRun: (projectId: string, runId: string) => void;
}

type TBuilder = (context: ISlideContext) => Omit<ISlide, 'id'>;

const BUILDERS: Record<Exclude<TStaticSlideId, 'theme-edit'>, TBuilder> = {
  'runner-new': ({ t, close }) => ({
    title: t('runners.new.title'),
    subtitle: <span>{t('runners.new.subtitle')}</span>,
    children: <LazyRunnerNewBody onDone={close} />,
  }),
  'organization-invite': ({ t, close }) => ({
    title: t('invite.title'),
    subtitle: <span>{t('invite.subtitle')}</span>,
    children: <LazyInviteBody />,
    footer: <LazyInviteFooter onClose={close} />,
  }),
  'organization-transfer': ({ t, close }) => ({
    title: t('organization.transfer.title'),
    subtitle: <span>{t('organization.transfer.subtitle')}</span>,
    children: <LazyTransferBody onDone={close} />,
  }),
  'organization-token-new': ({ t, close }) => ({
    title: t('tokens.slide.title'),
    subtitle: <span>{t('orgTokens.new.subtitle')}</span>,
    children: <LazyOrganizationTokenNewBody onDone={close} />,
  }),
  'organization-new': ({ t, openOrganization }) => ({
    title: t('organization.new.title'),
    subtitle: <span>{t('organization.new.subtitle')}</span>,
    children: <LazyOrganizationNewBody onCreated={openOrganization} />,
  }),
  'team-new': ({ t, openTeam }) => ({
    title: t('team.new.title'),
    subtitle: <span>{t('team.new.subtitle')}</span>,
    children: <LazyTeamNewBody onCreated={openTeam} />,
  }),
  'account-password': ({ t, close }) => ({
    title: t('password.slide.title'),
    subtitle: <span>{t('password.slide.subtitle')}</span>,
    children: <LazyPasswordBody onDone={close} />,
    footer: <LazyPasswordFooter onCancel={close} />,
  }),
  'account-totp': ({ t, close }) => ({
    title: t('totp.slide.title'),
    subtitle: <span>{t('totp.slide.subtitle')}</span>,
    children: <LazyTotpBody onDone={close} />,
    footer: <LazyTotpFooter onCancel={close} />,
  }),
  'member-remove': ({ t, close, memberId, openMembers }) => ({
    title: t('member.remove.title'),
    subtitle: <span>{t('member.remove.subtitle')}</span>,
    children: <LazyMemberRemoveBody userId={memberId} onDone={openMembers} onCancel={close} />,
  }),
  'organization-leave': ({ t, close, openHome }) => ({
    title: t('organization.leave.title'),
    subtitle: <span>{t('organization.leave.subtitle')}</span>,
    children: <LazyLeaveBody onLeft={openHome} onCancel={close} />,
  }),
  'oidc-new': ({ t, close }) => ({
    title: t('settings.oidc.new.title'),
    subtitle: <span>{t('settings.oidc.new.subtitle')}</span>,
    children: <LazyOidcProviderBody onDone={close} />,
  }),
  'project-new': ({ t, openProject }) => ({
    title: t('projects.new.title'),
    subtitle: <span>{t('projects.new.subtitle')}</span>,
    children: <LazyProjectNewBody onCreated={openProject} />,
  }),
  'repository-link': ({ t, close, projectId }) => ({
    title: t('repository.link.title'),
    subtitle: <span>{t('repository.link.subtitle')}</span>,
    children: <LazyRepositoryLinkBody projectId={projectId} onDone={close} />,
  }),
  'run-trigger': ({ t, projectId, openRun }) => ({
    title: t('runs.trigger.title'),
    subtitle: <span>{t('runs.trigger.subtitle')}</span>,
    children: (
      <LazyTriggerRunBody
        projectId={projectId}
        onStarted={(runId) => {
          if (projectId !== undefined) openRun(projectId, runId);
        }}
      />
    ),
  }),
  'secret-new': ({ t, close, projectId }) => ({
    title: t('secrets.new.title'),
    subtitle: <span>{t('secrets.new.subtitle')}</span>,
    children: <LazySecretFormBody projectId={projectId} onDone={close} />,
  }),
  'variable-new': ({ t, close, projectId }) => ({
    title: t('variables.new.title'),
    subtitle: <span>{t('variables.new.subtitle')}</span>,
    children: <LazyVariableFormBody projectId={projectId} onDone={close} />,
  }),
  'registry-new': ({ t, close, projectId }) => ({
    title: t('registry.new.title'),
    subtitle: <span>{t('registry.new.subtitle')}</span>,
    children: <LazyRegistryFormBody projectId={projectId} onDone={close} />,
  }),
  'account-token-new': ({ t, close }) => ({
    title: t('tokens.slide.title'),
    subtitle: <span>{t('tokens.slide.subtitle')}</span>,
    children: <LazyTokenNewBody onDone={close} />,
  }),
};

const themeEditSlide = (id: TSlideId, { t, close, draftName, customThemes }: ISlideContext): ISlide => {
  const sourceId = themeOf(id) ?? undefined;
  const existing = customThemes.find((theme) => theme.Id === sourceId)?.Name;
  return {
    id,
    title:
      [draftName?.trim(), existing].find((name) => name !== undefined && name !== '') ?? t('themeEditor.title.new'),
    subtitle: <span>{t('themeEditor.subtitle')}</span>,
    children: <LazyThemeEditorBody sourceId={sourceId} onDone={close} />,
    footer: <LazyThemeEditorFooter onCancel={close} onDone={close} />,
  };
};

const oidcEditSlide = (id: TSlideId, providerId: string, { t, close }: ISlideContext): ISlide => ({
  id,
  title: t('settings.oidc.edit.title', { name: providerId }),
  subtitle: <span>{t('settings.oidc.edit.subtitle')}</span>,
  children: <LazyOidcProviderBody providerId={providerId} onDone={close} />,
});

const secretsEditSlide = (id: TSlideId, target: ISecretsEditTarget, { t, close, projectId }: ISlideContext): ISlide => {
  if (target.Kind === 'secret') {
    return {
      id,
      title: t('secrets.edit.title', { name: target.Key }),
      subtitle: <span>{t('secrets.edit.subtitle')}</span>,
      children: <LazySecretFormBody projectId={projectId} name={target.Key} onDone={close} />,
    };
  }
  if (target.Kind === 'variable') {
    return {
      id,
      title: t('variables.edit.title', { name: target.Key }),
      subtitle: <span>{t('variables.edit.subtitle')}</span>,
      children: <LazyVariableFormBody projectId={projectId} name={target.Key} onDone={close} />,
    };
  }
  return {
    id,
    title: t('registry.edit.title'),
    subtitle: <span>{t('registry.edit.subtitle')}</span>,
    children: <LazyRegistryFormBody projectId={projectId} credentialId={target.Key} onDone={close} />,
  };
};

export const buildSlide = (id: TSlideId, context: ISlideContext): ISlide => {
  if (id === 'theme-edit' || themeOf(id) !== null) return themeEditSlide(id, context);
  const providerId = oidcOf(id);
  if (providerId !== null) return oidcEditSlide(id, providerId, context);
  const target = secretsTargetOf(id);
  if (target !== null) return secretsEditSlide(id, target, context);
  const builder = Object.hasOwn(BUILDERS, id) ? BUILDERS[id as keyof typeof BUILDERS] : undefined;
  return builder ? { id, ...builder(context) } : themeEditSlide(id, context);
};

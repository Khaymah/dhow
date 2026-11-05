import type { ReactNode } from 'react';

import { Card } from '@/Core/Ui/Card';
import { useT } from '@/I18n/useT';
import type { TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';

export interface IRulesSummaryProps {
  environment: TEnvironment;
}

const Fact = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="grid gap-0.5">
    <dt className="text-body-sm text-tertiary">{label}</dt>
    <dd className="m-0 min-w-0 text-body-sm text-primary">{children}</dd>
  </div>
);

const Mono = ({ children }: { children: string }) => (
  <bdi dir="ltr" className="font-mono">
    {children}
  </bdi>
);

export const RulesSummary = ({ environment }: IRulesSummaryProps) => {
  const t = useT();
  const rules = environment.Rules;
  const yesNo = (value: boolean) => (value ? t('environments.summary.yes') : t('environments.summary.no'));
  const reviewers = rules.Reviewers;
  const reviewerCount = reviewers.UserCount + reviewers.TeamCount;

  return (
    <Card title={t('environments.rules.title')} icon="shield" padded>
      <dl className="m-0 grid gap-x-4 gap-y-3 sm:grid-cols-2">
        <Fact label={t('environments.refs.title')}>
          {rules.AllowedRefs.length === 0 ? (
            t('environments.refs.none')
          ) : (
            <ul className="m-0 grid list-none gap-1 p-0">
              {rules.AllowedRefs.map((ref) => (
                <li key={`${ref.Kind}-${ref.Pattern}`}>
                  {t(`environments.refs.kind.${ref.Kind}`)} <Mono>{ref.Pattern}</Mono>
                </li>
              ))}
            </ul>
          )}
        </Fact>
        <Fact label={t('environments.rule.pullRequests')}>{yesNo(rules.AllowPullRequests)}</Fact>
        <Fact label={t('environments.reviewers.title')}>
          {reviewerCount === 0
            ? t('environments.summary.reviewers.none')
            : t('environments.summary.reviewers', { users: reviewers.UserCount, teams: reviewers.TeamCount })}
        </Fact>
        <Fact label={t('environments.rule.selfApproval')}>{yesNo(rules.AllowSelfApproval)}</Fact>
        <Fact label={t('environments.rule.wait')}>{t('environments.summary.minutes', { count: rules.WaitMinutes })}</Fact>
        <Fact label={t('environments.rule.timeout')}>{t('environments.summary.hours', { count: rules.ApprovalTimeoutHours })}</Fact>
        <Fact label={t('environments.rule.pinned')}>{yesNo(rules.RequirePinnedImages)}</Fact>
        <Fact label={t('environments.rule.plugins')}>
          {rules.AllowedPlugins.length === 0 ? (
            t('environments.summary.plugins.any')
          ) : (
            <ul className="m-0 grid list-none gap-1 p-0">
              {rules.AllowedPlugins.map((plugin) => (
                <li key={plugin}>
                  <Mono>{plugin}</Mono>
                </li>
              ))}
            </ul>
          )}
        </Fact>
        <Fact label={t('environments.rule.bypass')}>{yesNo(rules.AllowAdminBypass)}</Fact>
        <Fact label={t('environments.rule.supersede')}>{yesNo(rules.SupersedePending)}</Fact>
      </dl>
    </Card>
  );
};

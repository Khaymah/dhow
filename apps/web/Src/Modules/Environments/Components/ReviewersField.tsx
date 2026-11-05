import { Button } from '@/Core/Ui/Button';
import { CheckList, type ICheckItem } from '@/Core/Ui/CheckList';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { useReviewerMembers, useReviewerTeams } from '@/Modules/Environments/Hooks/useEnvironments';
import { MAX_REVIEWERS } from '@/Modules/Environments/Types/EnvironmentRules';

export interface IReviewersFieldProps {
  organizationId: string;
  userIds: string[];
  teamIds: string[];
  disabled: boolean;
  onUsers: (ids: string[]) => void;
  onTeams: (ids: string[]) => void;
}

const ELIGIBLE_ROLES = ['owner', 'admin', 'maintainer', 'developer'];

const withUnknown = (items: ICheckItem<string>[], selected: string[], unknown: string): ICheckItem<string>[] => [
  ...items,
  ...selected.filter((id) => !items.some((item) => item.value === id)).map((id) => ({ value: id, title: unknown, about: id })),
];

export const ReviewersField = ({ organizationId, userIds, teamIds, disabled, onUsers, onTeams }: IReviewersFieldProps) => {
  const t = useT();
  const members = useReviewerMembers(organizationId, true);
  const teams = useReviewerTeams(organizationId, true);

  if (members.isError || teams.isError) {
    return (
      <div className="grid justify-items-start gap-2">
        <InlineAlert accent="failed" title={t('environments.reviewers.error')} />
        <Button
          size="sm"
          onClick={() => {
            void members.refetch();
            void teams.refetch();
          }}
        >
          {t('status.retry')}
        </Button>
      </div>
    );
  }
  if (members.data === undefined || teams.data === undefined) return <Skeleton label={t('environments.reviewers.loading')} lines={3} />;

  const memberItems = withUnknown(
    members.data
      .filter((member) => ELIGIBLE_ROLES.includes(member.EffectiveRole))
      .map((member) => ({ value: member.UserId, title: member.Name, about: member.Email })),
    userIds,
    t('environments.reviewers.unknown'),
  );
  const teamItems = withUnknown(
    teams.data.map((team) => ({ value: team.Id, title: team.Name, about: team.Description })),
    teamIds,
    t('environments.reviewers.unknown'),
  );

  return (
    <div className="grid gap-3">
      <div className="grid gap-1">
        <span className="text-body-sm text-secondary">{t('environments.reviewers.title')}</span>
        <small className="text-body-sm text-tertiary">{t('environments.reviewers.about', { max: MAX_REVIEWERS })}</small>
      </div>
      {memberItems.length === 0 ? (
        <p className="m-0 text-body-sm text-secondary">{t('environments.reviewers.noMembers')}</p>
      ) : (
        <CheckList label={t('environments.reviewers.users')} items={memberItems} values={userIds} onChange={onUsers} disabled={disabled} />
      )}
      {teamItems.length === 0 ? (
        <p className="m-0 text-body-sm text-secondary">{t('environments.reviewers.noTeams')}</p>
      ) : (
        <CheckList label={t('environments.reviewers.teams')} items={teamItems} values={teamIds} onChange={onTeams} disabled={disabled} />
      )}
    </div>
  );
};

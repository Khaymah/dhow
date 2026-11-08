import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Card } from '@/Core/Ui/Card';
import { Row, Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import { CheckBadge } from '@/Modules/System/Components/CheckBadge';
import type { TServerBuild } from '@/Modules/System/Types/SystemTypes';

const Mono = ({ children }: { children: string }) => (
  <bdi dir="ltr" className="font-mono text-code text-primary">
    {children}
  </bdi>
);

export interface IAboutVersionCardProps {
  build: TServerBuild;
  healthy: boolean;
}

export const AboutVersionCard = ({ build, healthy }: IAboutVersionCardProps) => {
  const t = useT();
  const time = useRelativeTime();

  return (
    <Card title={t('about.build.title')} icon="info">
      <Rows>
        <Row title={t('status.build.version')} end={<Mono>{build.Version}</Mono>} />
        <Row title={t('status.build.commit')} end={<Mono>{build.Commit === '' ? t('about.build.unknown') : build.Commit}</Mono>} />
        <Row
          title={t('about.build.date')}
          end={build.BuiltAt === undefined ? t('about.build.unknown') : <time dateTime={build.BuiltAt}>{time.full(build.BuiltAt)}</time>}
        />
        <Row title={t('about.build.health')} end={<CheckBadge ok={healthy} />} />
      </Rows>
    </Card>
  );
};

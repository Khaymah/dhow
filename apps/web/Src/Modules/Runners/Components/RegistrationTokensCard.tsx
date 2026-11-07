import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Row, Rows } from '@/Core/Ui/Row';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { TOKENS_PAGE_SIZE, type TRegistrationToken } from '@/Modules/Runners/Api/RunnersApi';
import { useRegistrationTokens, useRevokeRegistrationToken } from '@/Modules/Runners/Hooks/useRunners';
import { TOKEN_LABEL, TOKEN_TONE } from '@/Modules/Runners/Types/RunnerView';

export interface IRegistrationTokensCardProps {
  organizationId: string;
}

interface ITokenRowProps {
  token: TRegistrationToken;
  onAskRevoke: () => void;
}

const TokenRow = ({ token, onAskRevoke }: ITokenRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const name = token.Description === '' ? t('runners.token.noDescription') : token.Description;
  const labels = token.Labels.length === 0 ? t('runners.labels.none') : <bdi dir="ltr">{token.Labels.join(' · ')}</bdi>;
  const uses = t('runners.token.uses', { uses: token.Uses, max: token.MaxUses });
  const when =
    token.Status === 'active' ? t('runners.token.expires', { when: time.full(token.ExpiresAt) }) : t('runners.token.created', { when: time.ago(token.CreatedAt) });

  return (
    <Row
      icon="key"
      title={name}
      sub={
        <>
          {labels}
          {` · ${uses} · ${when}`}
        </>
      }
      end={
        <>
          <Badge tone={TOKEN_TONE[token.Status]}>{t(TOKEN_LABEL[token.Status])}</Badge>
          {token.Status === 'active' ? (
            <Button size="sm" variant="ghost" iconStart="trash" aria-label={t('runners.token.revoke.aria', { name })} onClick={onAskRevoke}>
              <span className="max-sm:sr-only">{t('runners.token.revoke')}</span>
            </Button>
          ) : null}
        </>
      }
    />
  );
};

export const RegistrationTokensCard = ({ organizationId }: IRegistrationTokensCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const Push = useToastStore((state) => state.Push);
  const [Page, setPage] = useState(1);
  const [Asking, setAsking] = useState<string | null>(null);
  const tokens = useRegistrationTokens(organizationId, Page);
  const revoke = useRevokeRegistrationToken(organizationId);

  if (tokens.isError) {
    return (
      <InlineAlert accent="failed" title={t('runners.tokens.error')}>
        {errorText(tokens.error)}
      </InlineAlert>
    );
  }
  if (tokens.data === undefined) return <Skeleton label={t('runners.tokens.loading')} lines={2} />;

  const { Items, Meta } = tokens.data;
  return (
    <Card title={t('runners.tokens.title')} icon="key" end={Meta.Total > 0 ? String(Meta.Total) : undefined}>
      <p className="m-0 px-4 pb-2 text-body-sm text-secondary">{t('runners.tokens.lead')}</p>
      {revoke.error ? <ErrorAlert error={revoke.error} className="mx-4 mb-2 w-auto" /> : null}
      {Items.length === 0 ? <p className="m-0 px-4 pb-3 text-body-sm text-tertiary">{t('runners.tokens.empty')}</p> : null}
      <Rows label={t('runners.tokens.title')}>
        {Items.map((token) => [
          <TokenRow
            key={token.Id}
            token={token}
            onAskRevoke={() => {
              revoke.reset();
              setAsking(token.Id);
            }}
          />,
          Asking === token.Id && token.Status === 'active' ? (
            <li key={`${token.Id}-revoke`} className="grid gap-3 px-4 pb-4">
              <p className="m-0 text-body-sm text-secondary">{t('runners.token.revoke.question')}</p>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="danger"
                  iconStart="trash"
                  disabled={revoke.isPending}
                  onClick={() => {
                    revoke.mutate(token.Id, {
                      onSuccess: () => {
                        setAsking(null);
                        Push({ Id: `runner-token-revoked-${token.Id}`, Accent: 'live', Title: t('runners.token.revoked') });
                      },
                    });
                  }}
                >
                  {t('runners.token.revoke.confirm')}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={revoke.isPending}
                  onClick={() => {
                    setAsking(null);
                  }}
                >
                  {t('runners.cancel')}
                </Button>
              </div>
            </li>
          ) : null,
        ])}
      </Rows>
      {Meta.Total > Meta.Limit ? (
        <div className="px-4 pb-3">
          <Pager page={Meta.Page} limit={Meta.Limit || TOKENS_PAGE_SIZE} total={Meta.Total} busy={tokens.isFetching} onPage={setPage} />
        </div>
      ) : null}
    </Card>
  );
};

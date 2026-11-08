import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { PasswordInput } from '@/Core/Ui/PasswordInput';
import { Select } from '@/Core/Ui/Select';
import { useT } from '@/I18n/useT';
import { kindLabel, KINDS, needsBot, needsUrl, type IChannelDraft } from '@/Modules/Notifications/Types/NotificationsForm';
import type { TNotificationKind } from '@/Modules/Notifications/Api/NotificationsApi';

export interface IChannelFieldsProps {
  draft: IChannelDraft;
  disabled: boolean;
  onChange: (draft: IChannelDraft) => void;
}

const NAME_MAX = 64;

export const ChannelFields = ({ draft, disabled, onChange }: IChannelFieldsProps) => {
  const t = useT();

  return (
    <>
      <Field label={t('notifications.form.kind')}>
        <Select
          disabled={disabled}
          value={draft.Kind}
          options={KINDS.map((kind) => ({ value: kind, label: t(kindLabel(kind)) }))}
          onChange={(event) => {
            onChange({ ...draft, Kind: event.target.value as TNotificationKind });
          }}
        />
      </Field>
      <small className="-mt-3 text-body-sm text-tertiary">{t(`notifications.kind.${draft.Kind}.about`)}</small>
      <Field label={t('notifications.form.name')}>
        <Input
          maxLength={NAME_MAX}
          autoComplete="off"
          disabled={disabled}
          value={draft.Name}
          onChange={(event) => {
            onChange({ ...draft, Name: event.target.value });
          }}
        />
      </Field>
      <Field label={t('notifications.form.locale')}>
        <Select
          disabled={disabled}
          value={draft.Locale}
          options={[
            { value: 'ar', label: t('language.ar') },
            { value: 'en', label: t('language.en') },
          ]}
          onChange={(event) => {
            onChange({ ...draft, Locale: event.target.value === 'en' ? 'en' : 'ar' });
          }}
        />
      </Field>
      {needsUrl(draft.Kind) ? (
        <Field label={t('notifications.form.url')}>
          <PasswordInput
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
            value={draft.Url}
            onChange={(event) => {
              onChange({ ...draft, Url: event.target.value });
            }}
          />
        </Field>
      ) : null}
      {needsBot(draft.Kind) ? (
        <>
          <Field label={t('notifications.form.bot')}>
            <PasswordInput
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              value={draft.BotToken}
              onChange={(event) => {
                onChange({ ...draft, BotToken: event.target.value });
              }}
            />
          </Field>
          <Field label={t('notifications.form.chat')}>
            <Input
              dir="ltr"
              autoComplete="off"
              disabled={disabled}
              value={draft.ChatId}
              onChange={(event) => {
                onChange({ ...draft, ChatId: event.target.value });
              }}
            />
          </Field>
        </>
      ) : null}
    </>
  );
};

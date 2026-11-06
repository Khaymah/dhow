import { Button } from '@/Core/Ui/Button';
import { IconButton } from '@/Core/Ui/IconButton';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import type { ITargetForm } from '@/Modules/Environments/Hooks/useTargetDraft';
import { MAX_HOSTS } from '@/Modules/Environments/Types/TargetDraft';

export interface IHostRowsFieldProps {
  form: ITargetForm;
  disabled: boolean;
}

export const HostRowsField = ({ form, disabled }: IHostRowsFieldProps) => {
  const t = useT();
  const hosts = form.Draft.Hosts;

  return (
    <fieldset className="m-0 grid gap-2 border-0 p-0" disabled={disabled}>
      <legend className="p-0 text-body-sm text-secondary">{t('targets.form.hosts')}</legend>
      <small className="text-body-sm text-tertiary">{t('targets.form.hosts.about')}</small>
      {hosts.map((host, index) => (
        <div key={String(index)} className="flex min-w-0 items-center gap-2">
          <span className="w-6 flex-none text-body-sm text-tertiary">{index + 1}</span>
          <Input
            compact
            dir="ltr"
            className="font-mono"
            aria-label={t('targets.form.host')}
            maxLength={253}
            autoComplete="off"
            spellCheck={false}
            value={host.Host}
            onChange={(event) => {
              form.SetHost(index, { Host: event.target.value });
            }}
          />
          <Input
            compact
            dir="ltr"
            inputMode="numeric"
            className="w-24 flex-none font-mono"
            aria-label={t('targets.form.port')}
            maxLength={5}
            autoComplete="off"
            value={host.Port}
            onChange={(event) => {
              form.SetHost(index, { Port: event.target.value });
            }}
          />
          <IconButton
            size="sm"
            icon="trash"
            label={t('targets.form.host.remove')}
            disabled={hosts.length <= 1}
            onClick={() => {
              form.RemoveHost(index);
            }}
          />
        </div>
      ))}
      <Button size="sm" iconStart="plus" className="justify-self-start" disabled={hosts.length >= MAX_HOSTS} onClick={form.AddHost}>
        {t('targets.form.host.add')}
      </Button>
    </fieldset>
  );
};

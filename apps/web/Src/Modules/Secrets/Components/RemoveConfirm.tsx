import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';

export interface IRemoveConfirmProps {
  question: string;
  confirm: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export const RemoveConfirm = ({ question, confirm, busy, onConfirm, onCancel }: IRemoveConfirmProps) => {
  const t = useT();
  return (
    <li className="grid gap-3 px-card pb-card">
      <p className="m-0 text-body-sm text-secondary">{question}</p>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="danger" iconStart="trash" disabled={busy} onClick={onConfirm}>
          {confirm}
        </Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={onCancel}>
          {t('mfa.cancel')}
        </Button>
      </div>
    </li>
  );
};

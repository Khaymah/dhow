import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { copyText } from '@/Core/Ui/Clipboard';
import { useT } from '@/I18n/useT';

export interface ICopyFieldProps {
  id: string;
  label: string;
  value: string;
  copy: string;
  copied: string;
  primary?: boolean;
}

export const CopyField = ({ id, label, value, copy, copied, primary = false }: ICopyFieldProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);

  return (
    <div className="grid min-w-0 gap-2">
      <span className="text-body-sm text-secondary">{label}</span>
      <code dir="ltr" aria-label={label} className="block min-w-0 rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code break-all select-all">
        {value}
      </code>
      <Button
        size="sm"
        variant={primary ? 'primary' : 'secondary'}
        iconStart="copy"
        className="justify-self-start"
        onClick={() => {
          void copyText(value).then((done) => {
            Push(
              done
                ? { Id: `copied-${id}`, Accent: 'live', Title: copied }
                : { Id: `copy-failed-${id}`, Accent: 'awaiting', Title: t('projects.copy.failed') },
            );
          });
        }}
      >
        {copy}
      </Button>
    </div>
  );
};

import { useT } from '@/I18n/useT';

export interface IErrorHelpProps {
  fix: string;
  helpHref: string | null;
  fixId: string;
}

export const ErrorHelp = ({ fix, helpHref, fixId }: IErrorHelpProps) => {
  const t = useT();
  return (
    <span className="grid gap-0.5 text-body-sm text-secondary">
      {fix === '' ? null : (
        <span id={fixId}>
          <span className="text-primary">{t('api.error.fix')}</span> {fix}
        </span>
      )}
      {helpHref ? (
        <a
          href={helpHref}
          target="_blank"
          rel="noreferrer noopener"
          aria-describedby={fix === '' ? undefined : fixId}
          className="w-fit text-primary underline underline-offset-4"
        >
          {t('api.error.help')}{' '}
          <span aria-hidden="true">{'↗'}</span>
          <span className="sr-only">{`(${t('api.error.newTab')})`}</span>
        </a>
      ) : null}
    </span>
  );
};

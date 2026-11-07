import { useApiErrorHelp } from '@/Core/Hooks/useApiErrorHelp';
import { useApiErrorMessage } from '@/Core/Hooks/useApiErrorMessage';
import { InlineAlert } from '@/Core/Ui/InlineAlert';

export interface IErrorAlertProps {
  error: unknown;
  focus?: boolean;
  className?: string;
}

export const ErrorAlert = ({ error, focus = false, className }: IErrorAlertProps) => {
  const message = useApiErrorMessage();
  const help = useApiErrorHelp();
  const { Fix, HelpHref } = help(error);
  return <InlineAlert accent="failed" title={message(error)} focus={focus} className={className} fix={Fix} helpHref={HelpHref} />;
};

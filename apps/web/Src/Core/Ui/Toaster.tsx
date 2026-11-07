import { useEffect } from 'react';

import { type IToast, useToastStore } from '@/Core/Store/useToastStore';
import { Toast } from '@/Core/Ui/Toast';

const AUTO_DISMISS_MS = 6000;

export interface IToasterProps {
  dismissLabel: string;
}

interface IToastItemProps extends IToasterProps {
  toast: IToast;
}

const ToastItem = ({ toast, dismissLabel }: IToastItemProps) => {
  const Dismiss = useToastStore((state) => state.Dismiss);

  useEffect(() => {
    const timer = setTimeout(() => {
      Dismiss(toast.Id);
    }, AUTO_DISMISS_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [Dismiss, toast.Id]);

  return (
    <Toast
      accent={toast.Accent}
      title={toast.Title}
      detail={toast.Detail}
      fix={toast.Fix}
      helpHref={toast.HelpHref}
      dismissLabel={dismissLabel}
      onDismiss={() => {
        Dismiss(toast.Id);
      }}
    />
  );
};

export const Toaster = ({ dismissLabel }: IToasterProps) => {
  const Toasts = useToastStore((state) => state.Toasts);

  return (
    <div className="fixed bottom-4 end-4 z-[60] grid gap-2">
      {Toasts.map((toast) => (
        <ToastItem key={toast.Id} toast={toast} dismissLabel={dismissLabel} />
      ))}
    </div>
  );
};

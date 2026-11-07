import { useEffect, useId, useRef, type ReactNode } from "react";

import {
  ACCENT_ICON,
  ACCENT_SURFACE,
  ACCENT_TEXT,
  type TAccent,
} from "@/Core/Ui/Accent";
import { cx } from "@/Core/Ui/ClassNames";
import { ErrorHelp } from "@/Core/Ui/ErrorHelp";
import { Icon } from "@/Core/Ui/Icon";

export interface IInlineAlertProps {
  accent?: TAccent;
  title: string;
  children?: ReactNode;
  className?: string;
  focus?: boolean;
  fix?: string;
  helpHref?: string | null;
}

export const InlineAlert = ({
  accent = "building",
  title,
  children,
  className,
  focus = false,
  fix = "",
  helpHref = null,
}: IInlineAlertProps) => {
  const fixId = useId();
  const hasHelp = fix !== "" || helpHref !== null;
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (focus) ref.current?.focus();
  }, [focus, title]);

  return (
    <div
      ref={ref}
      tabIndex={focus ? -1 : undefined}
      role={accent === "failed" ? "alert" : "note"}
      aria-describedby={fix === "" ? undefined : fixId}
      data-accent={accent}
      className={cx(
        "box-border flex w-full gap-3 rounded-md border px-4 py-3 text-body-sm",
        ACCENT_SURFACE[accent],
        className,
      )}
    >
      <Icon
        name={ACCENT_ICON[accent]}
        size={17}
        className={cx("mt-[3px]", ACCENT_TEXT[accent])}
      />
      <span className="grid gap-0.5">
        {title}
        {children ? (
          <span className="text-body-sm text-secondary">{children}</span>
        ) : null}
        {hasHelp ? (
          <ErrorHelp fix={fix} helpHref={helpHref} fixId={fixId} />
        ) : null}
      </span>
    </div>
  );
};

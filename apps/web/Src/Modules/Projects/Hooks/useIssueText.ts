import type { z } from '@/Core/Forms/Zod';

import { isMessageKey } from '@/I18n/Translate';
import { useT } from '@/I18n/useT';

export type TIssues = Partial<Record<string, string>>;

export const useIssueText = (): ((error: z.ZodError) => TIssues) => {
  const t = useT();
  return (error) => {
    const out: TIssues = {};
    for (const issue of error.issues) {
      const path = issue.path.map(String).join('.');
      if (out[path] !== undefined) continue;
      out[path] = isMessageKey(issue.message) ? t(issue.message) : t('projects.form.error.invalid');
    }
    return out;
  };
};

import { useState } from 'react';

import type { IRulesDraft } from '@/Modules/Environments/Types/RulesDraft';

export interface IRulesForm {
  Draft: IRulesDraft;
  Patch: (changes: Partial<IRulesDraft>) => void;
  Reset: (next: IRulesDraft) => void;
}

export const useRulesDraft = (initial: IRulesDraft): IRulesForm => {
  const [Draft, setDraft] = useState<IRulesDraft>(initial);
  return {
    Draft,
    Patch: (changes) => {
      setDraft((current) => ({ ...current, ...changes }));
    },
    Reset: setDraft,
  };
};

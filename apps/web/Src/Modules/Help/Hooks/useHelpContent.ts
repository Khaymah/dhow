import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import type { IHelpContent } from '@/Core/Help/HelpTypes';
import { loadHelpContent } from '@/Core/Help/LoadHelpContent';
import { useI18n } from '@/I18n/useI18n';

export const useHelpContent = (enabled = true): UseQueryResult<IHelpContent> => {
  const { Locale } = useI18n();
  return useQuery({
    queryKey: ['help-content', Locale],
    queryFn: () => loadHelpContent(Locale),
    enabled,
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
};

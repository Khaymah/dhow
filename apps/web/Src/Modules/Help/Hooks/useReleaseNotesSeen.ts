import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { LATEST_RELEASE } from '@/Core/Help/Generated/ReleaseMeta';
import { isNewerRelease } from '@/Core/Help/ReleaseVersion';
import {
  getPreferences,
  PREFERENCES_QUERY_KEY,
  saveReleaseNotesSeen,
  type TPreferences,
} from '@/Core/Preferences/PreferencesApi';

export interface IReleaseNotesSeen {
  unseen: boolean;
  saving: boolean;
  failed: boolean;
  Dismiss: () => void;
}

export const useReleaseNotesSeen = (): IReleaseNotesSeen => {
  const client = useQueryClient();
  const preferences = useQuery<TPreferences>({
    queryKey: PREFERENCES_QUERY_KEY,
    queryFn: getPreferences,
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
  const save = useMutation({
    mutationFn: () => saveReleaseNotesSeen(LATEST_RELEASE),
    onSuccess: (saved) => {
      client.setQueryData(PREFERENCES_QUERY_KEY, saved);
    },
  });

  return {
    unseen: preferences.data !== undefined && isNewerRelease(LATEST_RELEASE, preferences.data.ReleaseNotesSeen),
    saving: save.isPending,
    failed: save.isError,
    Dismiss: () => {
      save.mutate();
    },
  };
};

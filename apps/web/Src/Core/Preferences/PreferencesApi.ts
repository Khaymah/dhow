import { ApiClient, request } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';

export type TPreferences = components['schemas']['Preferences'];
export type TSavePreferences = components['schemas']['SavePreferencesRequest'];

export const PREFERENCES_QUERY_KEY = ['preferences'] as const;

export const getPreferences = (): Promise<TPreferences> => request(() => ApiClient.GET('/me/preferences'));

export const savePreferences = (body: TSavePreferences): Promise<TPreferences> =>
  request(() => ApiClient.PUT('/me/preferences', { body }));

export const saveReleaseNotesSeen = (Version: string): Promise<TPreferences> =>
  request(() => ApiClient.PUT('/me/release-notes-seen', { body: { Version } }));

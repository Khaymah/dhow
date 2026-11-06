import { isApiError } from '@/Core/Api/ApiError';
import type { TMessageKey } from '@/I18n/Messages';
import type { TIngestRefKind, TIngestRefPattern } from '@/Modules/Ingest/Api/IngestApi';

export const TOKEN_NAME_MAX = 64;
export const PATTERN_MAX = 200;
export const MAX_PATTERNS = 20;
export const DEFAULT_DAYS = 90;
export const MAX_DAYS = 365;
export const PATTERN_EXAMPLES = ['main', 'release/*', 'release/**', 'v*'] as const;
export const REACH_UNCONFIRMED = 'INGEST_TOKEN_REACH_UNCONFIRMED';
export const PROTECTED_REACH_FIELD = 'ProtectedReach';

export interface IPatternDraft {
  Kind: TIngestRefKind;
  Pattern: string;
}

export interface ITokenDraft {
  Name: string;
  Patterns: IPatternDraft[];
  Days: string;
}

export const EMPTY_TOKEN_DRAFT: ITokenDraft = { Name: '', Patterns: [{ Kind: 'branch', Pattern: '' }], Days: String(DEFAULT_DAYS) };

export const daysOf = (value: string): number | null => {
  if (!/^\d{1,3}$/.test(value.trim())) return null;
  const days = Number(value.trim());
  return days >= 1 && days <= MAX_DAYS ? days : null;
};

export const patternsOf = (draft: ITokenDraft): TIngestRefPattern[] => {
  const seen = new Set<string>();
  const out: TIngestRefPattern[] = [];
  for (const item of draft.Patterns) {
    const Pattern = item.Pattern.trim();
    const key = `${item.Kind}:${Pattern}`;
    if (Pattern === '' || seen.has(key)) continue;
    seen.add(key);
    out.push({ Kind: item.Kind, Pattern });
  }
  return out;
};

export const draftProblem = (draft: ITokenDraft): TMessageKey | null => {
  const name = draft.Name.trim();
  if (name === '' || name.length > TOKEN_NAME_MAX) return 'ingest.form.error.name';
  const patterns = patternsOf(draft);
  if (patterns.length === 0 || patterns.length > MAX_PATTERNS) return 'ingest.form.error.patterns';
  if (patterns.some((item) => item.Pattern.length > PATTERN_MAX || /\s/.test(item.Pattern))) return 'ingest.form.error.pattern';
  if (daysOf(draft.Days) === null) return 'ingest.form.error.days';
  return null;
};

export const protectedReachOf = (error: unknown): string[] | null => {
  if (!isApiError(error) || error.Code !== REACH_UNCONFIRMED) return null;
  return error.Details.filter((detail) => detail.Field === PROTECTED_REACH_FIELD).map((detail) => detail.Message);
};

export const INGEST_GUIDE_URL = {
  ar: 'https://github.com/Khaymah/dhow/blob/main/docs/guides/ingest.md',
  en: 'https://github.com/Khaymah/dhow/blob/main/docs/en/guides/ingest.md',
} as const;

export const INGEST_ACTION_URL = {
  ar: 'https://github.com/Khaymah/dhow/blob/main/docs/guides/ingest-action.md',
  en: 'https://github.com/Khaymah/dhow/blob/main/docs/en/guides/ingest-action.md',
} as const;

export const ACTION_SNIPPET = [
  '- id: dhow',
  '  uses: Khaymah/dhow-action@v1.0.0',
  '  with:',
  '    server: https://dhow.example.com',
  '    token: ${{ secrets.DHOW_INGEST_TOKEN }}',
  '    path: dist/web',
  '    name: web',
  '- env:',
  '    DIGEST: ${{ steps.dhow.outputs.digest }}',
  '  run: echo "Dhow stored sha256:$DIGEST"',
].join('\n');

export const CLI_SNIPPET = [
  'export DHOW_INGEST_TOKEN="<your-ingest-token>"',
  'dhow artifact push \\',
  '  --server https://dhow.example.com \\',
  '  --name web --ref main --sha "$GIT_SHA" \\',
  '  -- dist/web',
].join('\n');

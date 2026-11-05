import type { TMessageKey } from '@/I18n/Messages';
import type { TEnvironment, TEnvironmentRulesInput, TRefKind } from '@/Modules/Environments/Api/EnvironmentsApi';
import {
  DEFAULT_TIMEOUT_HOURS,
  DESCRIPTION_MAX,
  hasInvisible,
  isWholeNumber,
  MAX_PLUGINS,
  MAX_REFS,
  MAX_REVIEWERS,
  PLUGIN_MAX,
  patternProblem,
  TIMEOUT_MAX,
  TIMEOUT_MIN,
  WAIT_MAX,
} from '@/Modules/Environments/Types/EnvironmentRules';

export interface IRefDraft {
  Kind: TRefKind;
  Pattern: string;
}

export interface IRulesDraft {
  Description: string;
  Refs: IRefDraft[];
  RefsEdited: boolean;
  AllowPullRequests: boolean;
  UserIds: string[];
  TeamIds: string[];
  AllowSelfApproval: boolean;
  WaitMinutes: string;
  ApprovalTimeoutHours: string;
  RequirePinnedImages: boolean;
  Plugins: string;
  AllowAdminBypass: boolean;
  SupersedePending: boolean;
}

export const EMPTY_DRAFT: IRulesDraft = {
  Description: '',
  Refs: [],
  RefsEdited: false,
  AllowPullRequests: false,
  UserIds: [],
  TeamIds: [],
  AllowSelfApproval: false,
  WaitMinutes: '0',
  ApprovalTimeoutHours: String(DEFAULT_TIMEOUT_HOURS),
  RequirePinnedImages: false,
  Plugins: '',
  AllowAdminBypass: false,
  SupersedePending: true,
};

export const draftOf = (environment: TEnvironment): IRulesDraft => {
  const rules = environment.Rules;
  return {
    Description: environment.Description,
    Refs: rules.AllowedRefs.map((ref) => ({ Kind: ref.Kind, Pattern: ref.Pattern })),
    RefsEdited: false,
    AllowPullRequests: rules.AllowPullRequests,
    UserIds: [...rules.Reviewers.UserIds],
    TeamIds: [...rules.Reviewers.TeamIds],
    AllowSelfApproval: rules.AllowSelfApproval,
    WaitMinutes: String(rules.WaitMinutes),
    ApprovalTimeoutHours: String(rules.ApprovalTimeoutHours),
    RequirePinnedImages: rules.RequirePinnedImages,
    Plugins: rules.AllowedPlugins.join('\n'),
    AllowAdminBypass: rules.AllowAdminBypass,
    SupersedePending: rules.SupersedePending,
  };
};

export const pluginList = (text: string): string[] =>
  text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');

const sameList = (left: readonly string[], right: readonly string[]): boolean =>
  left.length === right.length && left.every((item, index) => item === right[index]);

const sameSet = (left: readonly string[], right: readonly string[]): boolean =>
  left.length === right.length && left.every((item) => right.includes(item));

const refLines = (refs: readonly IRefDraft[]): string[] => refs.map((ref) => `${ref.Kind}:${ref.Pattern.trim()}`);

const cleanRefs = (refs: readonly IRefDraft[]): IRefDraft[] => refs.map((ref) => ({ Kind: ref.Kind, Pattern: ref.Pattern.trim() }));

export const hasDraftChanges = (draft: IRulesDraft, original: IRulesDraft, reviewersEditable: boolean): boolean =>
  draft.Description !== original.Description ||
  !sameList(refLines(draft.Refs), refLines(original.Refs)) ||
  draft.AllowPullRequests !== original.AllowPullRequests ||
  (reviewersEditable && (!sameSet(draft.UserIds, original.UserIds) || !sameSet(draft.TeamIds, original.TeamIds))) ||
  draft.AllowSelfApproval !== original.AllowSelfApproval ||
  draft.WaitMinutes.trim() !== original.WaitMinutes.trim() ||
  draft.ApprovalTimeoutHours.trim() !== original.ApprovalTimeoutHours.trim() ||
  draft.RequirePinnedImages !== original.RequirePinnedImages ||
  !sameList(pluginList(draft.Plugins), pluginList(original.Plugins)) ||
  draft.AllowAdminBypass !== original.AllowAdminBypass ||
  draft.SupersedePending !== original.SupersedePending;

export const rulesInputOf = (draft: IRulesDraft, original: IRulesDraft, creating: boolean, reviewersEditable: boolean): TEnvironmentRulesInput => {
  const out: TEnvironmentRulesInput = {};
  const refsChanged = !sameList(refLines(draft.Refs), refLines(original.Refs));
  if (refsChanged || (creating && draft.RefsEdited)) out.AllowedRefs = cleanRefs(draft.Refs);
  if (draft.AllowPullRequests !== original.AllowPullRequests) out.AllowPullRequests = draft.AllowPullRequests;
  if (reviewersEditable && (!sameSet(draft.UserIds, original.UserIds) || !sameSet(draft.TeamIds, original.TeamIds))) {
    out.Reviewers = { UserIds: draft.UserIds, TeamIds: draft.TeamIds };
  }
  if (draft.AllowSelfApproval !== original.AllowSelfApproval) out.AllowSelfApproval = draft.AllowSelfApproval;
  if (draft.WaitMinutes.trim() !== original.WaitMinutes.trim()) out.WaitMinutes = Number(draft.WaitMinutes.trim());
  if (draft.ApprovalTimeoutHours.trim() !== original.ApprovalTimeoutHours.trim()) {
    out.ApprovalTimeoutHours = Number(draft.ApprovalTimeoutHours.trim());
  }
  if (draft.RequirePinnedImages !== original.RequirePinnedImages) out.RequirePinnedImages = draft.RequirePinnedImages;
  if (!sameList(pluginList(draft.Plugins), pluginList(original.Plugins))) out.AllowedPlugins = pluginList(draft.Plugins);
  if (draft.AllowAdminBypass !== original.AllowAdminBypass) out.AllowAdminBypass = draft.AllowAdminBypass;
  if (draft.SupersedePending !== original.SupersedePending) out.SupersedePending = draft.SupersedePending;
  return out;
};

const rangeProblem = (text: string, min: number, max: number): boolean =>
  !isWholeNumber(text) || Number(text.trim()) < min || Number(text.trim()) > max;

export const draftProblem = (draft: IRulesDraft, reviewersEditable: boolean): TMessageKey | null => {
  if (draft.Description.length > DESCRIPTION_MAX || hasInvisible(draft.Description)) return 'environments.error.description';
  if (draft.Refs.length > MAX_REFS) return 'environments.error.refs.many';
  for (const ref of draft.Refs) {
    const problem = patternProblem(ref.Pattern);
    if (problem !== null) return `environments.error.pattern.${problem}`;
  }
  if (reviewersEditable && draft.UserIds.length > MAX_REVIEWERS) return 'environments.error.reviewers.many';
  if (reviewersEditable && draft.TeamIds.length > MAX_REVIEWERS) return 'environments.error.reviewers.many';
  if (rangeProblem(draft.WaitMinutes, 0, WAIT_MAX)) return 'environments.error.wait';
  if (rangeProblem(draft.ApprovalTimeoutHours, TIMEOUT_MIN, TIMEOUT_MAX)) return 'environments.error.timeout';
  const plugins = pluginList(draft.Plugins);
  if (plugins.length > MAX_PLUGINS) return 'environments.error.plugins.many';
  if (plugins.some((plugin) => plugin.length > PLUGIN_MAX || hasInvisible(plugin))) return 'environments.error.plugins.invalid';
  return null;
};

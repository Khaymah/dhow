import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction, type IFactorAction } from '@/Core/Hooks/useFactorAction';
import { createIngestToken, type TCreatedIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { daysOf, MAX_DAYS, patternsOf, protectedReachOf, type ITokenDraft } from '@/Modules/Ingest/Types/IngestForm';

export interface ICreateIngestTokenInput {
  OrganizationId: string;
  ProjectId: string;
  Draft: ITokenDraft;
  Acknowledged: boolean;
  OnReach: (names: string[]) => void;
}

export const useCreateIngestToken = (
  methods: readonly TFactorMethod[],
  input: ICreateIngestTokenInput,
): IFactorAction<TCreatedIngestToken> =>
  useFactorAction<TCreatedIngestToken>(methods, async (proof) => {
    const factor = await proof();
    try {
      return await createIngestToken(
        input.OrganizationId,
        input.ProjectId,
        {
          Name: input.Draft.Name.trim(),
          RefPatterns: patternsOf(input.Draft),
          ExpiresInDays: daysOf(input.Draft.Days) ?? MAX_DAYS,
          AcknowledgeProtectedReach: input.Acknowledged,
        },
        factor,
      );
    } catch (error) {
      const reached = protectedReachOf(error);
      if (reached !== null) input.OnReach(reached);
      throw error;
    }
  });

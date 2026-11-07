import { getJobLogPage, type TJobLogChunk } from '@/Modules/Jobs/Api/JobsApi';
import type { IOlderChunks } from '@/Modules/Jobs/Types/LogBuffer';

export const OLDER_SPAN = 2000;

export const fetchOlder = async (organizationId: string, jobId: string, attempt: number, head: number): Promise<IOlderChunks> => {
  const start = Math.max(1, head - OLDER_SPAN);
  const from = start > 1 ? start - 1 : 1;
  const chunks: TJobLogChunk[] = [];
  let next: number | undefined = from;
  while (next !== undefined && next < head) {
    const page = await getJobLogPage(organizationId, jobId, attempt, next);
    for (const chunk of page.Chunks) if (chunk.Seq < head) chunks.push(chunk);
    const following: number | undefined = page.NextSeq;
    next = following !== undefined && following > next ? following : undefined;
  }
  return { Chunks: chunks, ContextSeq: from < start ? from : null };
};

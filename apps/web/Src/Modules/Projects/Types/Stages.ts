import type { TRunJob } from '@/Modules/Projects/Api/RunsApi';

export interface IStage {
  Index: number;
  Jobs: TRunJob[];
}

const levelsOf = (jobs: readonly TRunJob[]): Map<string, number> => {
  const needs = new Map<string, readonly string[]>();
  for (const job of jobs) needs.set(job.JobId, job.Needs);
  const levels = new Map<string, number>();
  const visit = (id: string, path: ReadonlySet<string>): number => {
    const known = levels.get(id);
    if (known !== undefined) return known;
    if (path.has(id)) return 0;
    const next = new Set(path).add(id);
    const parents = (needs.get(id) ?? []).filter((parent) => needs.has(parent));
    const level = parents.length === 0 ? 0 : Math.max(...parents.map((parent) => visit(parent, next))) + 1;
    levels.set(id, level);
    return level;
  };
  for (const id of needs.keys()) visit(id, new Set());
  return levels;
};

export const stagesOf = (jobs: readonly TRunJob[]): IStage[] => {
  const levels = levelsOf(jobs);
  const stages: IStage[] = [];
  for (const job of jobs) {
    const index = levels.get(job.JobId) ?? 0;
    const stage = stages.find((entry) => entry.Index === index);
    if (stage === undefined) stages.push({ Index: index, Jobs: [job] });
    else stage.Jobs.push(job);
  }
  return stages.sort((a, b) => a.Index - b.Index);
};

export const matrixOf = (job: Pick<TRunJob, 'Matrix'>): string[] =>
  Object.entries(job.Matrix)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, value]) => `${name}=${value}`);

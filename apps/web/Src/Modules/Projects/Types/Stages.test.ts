import { describe, expect, it } from 'vitest';

import type { TRunJob } from '@/Modules/Projects/Api/RunsApi';
import { matrixOf, stagesOf } from '@/Modules/Projects/Types/Stages';

const job = (JobId: string, Needs: string[] = [], Matrix: Record<string, string> = {}, Key = JobId): TRunJob => ({
  Id: `id-${Key}`,
  Key,
  JobId,
  Name: JobId,
  Matrix,
  Needs,
  State: 'waiting',
  Attempts: 0,
});

describe('the run stages', () => {
  it('groups jobs by how deep they sit in the DAG, keeping the pipeline order inside a stage', () => {
    const jobs = [
      job('lint'),
      job('build', [], { node: '20' }, 'build (node=20)'),
      job('build', [], { node: '22' }, 'build (node=22)'),
      job('test', ['build']),
      job('deploy', ['test', 'lint']),
    ];
    const stages = stagesOf(jobs);
    expect(stages.map((stage) => stage.Jobs.map((entry) => entry.Key))).toEqual([
      ['lint', 'build (node=20)', 'build (node=22)'],
      ['test'],
      ['deploy'],
    ]);
  });

  it('ignores needs on unknown jobs and survives a cycle', () => {
    expect(stagesOf([job('a', ['ghost'])]).map((stage) => stage.Index)).toEqual([0]);
    expect(stagesOf([job('a', ['b']), job('b', ['a'])]).flatMap((stage) => stage.Jobs).length).toBe(2);
  });

  it('prints the matrix values sorted by name', () => {
    expect(matrixOf({ Matrix: { os: 'debian', node: '20' } })).toEqual(['node=20', 'os=debian']);
    expect(matrixOf({ Matrix: {} })).toEqual([]);
  });
});

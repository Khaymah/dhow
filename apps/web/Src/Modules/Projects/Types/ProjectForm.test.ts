import { describe, expect, it } from 'vitest';

import { linkFormSchema } from '@/Modules/Projects/Types/LinkForm';
import { projectFormSchema, slugOf } from '@/Modules/Projects/Types/ProjectForm';

describe('the project form', () => {
  it('makes a slug from the name', () => {
    expect(slugOf('Web App 2')).toBe('web-app-2');
    expect(slugOf('  --Café!! ')).toBe('cafe');
    expect(slugOf('تطبيق')).toBe('');
  });

  it('refuses a wrong slug and a path that leaves the repository', () => {
    const parsed = projectFormSchema.safeParse({ Name: 'Web', Slug: 'Web_App', PipelinePath: '../ci.yml' });
    expect(parsed.error?.issues.map((issue) => issue.message).sort()).toEqual(['projects.form.error.path', 'projects.form.error.slug']);
    for (const path of ['/abs.yml', 'ci\\dhow.yml', 'ci/dhow.txt']) {
      expect(projectFormSchema.safeParse({ Name: 'Web', Slug: 'web', PipelinePath: path }).success).toBe(false);
    }
    expect(projectFormSchema.safeParse({ Name: 'Web', Slug: 'web', PipelinePath: 'ci/dhow.yaml' }).success).toBe(true);
  });
});

describe('the repository address form', () => {
  it('takes SSH, scp-like and HTTPS addresses only', () => {
    for (const url of ['ssh://git@git.example.com:2222/team/app.git', 'git@git.example.com:team/app.git', 'https://git.example.com/team/app.git']) {
      expect(linkFormSchema.safeParse({ CloneUrl: url, DefaultBranch: 'main' }).success).toBe(true);
    }
    for (const url of ['file:///srv/app', 'ext::sh -c id', 'git.example.com/team/app']) {
      expect(linkFormSchema.safeParse({ CloneUrl: url, DefaultBranch: 'main' }).success).toBe(false);
    }
  });

  it('refuses branch names git would refuse', () => {
    for (const branch of ['-main', 'a..b', 'x.lock', 'a b', 'feature/']) {
      expect(linkFormSchema.safeParse({ CloneUrl: 'git@git.example.com:a/b.git', DefaultBranch: branch }).success).toBe(false);
    }
  });
});

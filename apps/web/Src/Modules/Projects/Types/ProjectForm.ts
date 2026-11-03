import { z } from '@/Core/Forms/Zod';

export const PROJECT_NAME_MAX = 100;
export const PROJECT_SLUG_MAX = 63;
export const PIPELINE_PATH_MAX = 255;
export const DEFAULT_PIPELINE_PATH = '.dhow.yml';
export const PIPELINE_EXTENSIONS = '.yml / .yaml';

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const PIPELINE_PATH = /^(?!\/)(?!.*\\)(?!.*(?:^|\/)\.\.(?:\/|$)).+\.ya?ml$/;

export const nameField = z
  .string()
  .trim()
  .min(1, { error: 'projects.form.error.name' })
  .max(PROJECT_NAME_MAX, { error: 'projects.form.error.name' });

export const pipelinePathField = z
  .string()
  .trim()
  .min(1, { error: 'projects.form.error.path' })
  .max(PIPELINE_PATH_MAX, { error: 'projects.form.error.path' })
  .regex(PIPELINE_PATH, { error: 'projects.form.error.path' });

export const projectFormSchema = z.object({
  Name: nameField,
  Slug: z.string().trim().min(1, { error: 'projects.form.error.slug' }).max(PROJECT_SLUG_MAX, { error: 'projects.form.error.slug' }).regex(SLUG, { error: 'projects.form.error.slug' }),
  PipelinePath: pipelinePathField,
});

export type TProjectForm = z.infer<typeof projectFormSchema>;

export const slugOf = (name: string): string =>
  name
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, PROJECT_SLUG_MAX)
    .replace(/-+$/g, '');

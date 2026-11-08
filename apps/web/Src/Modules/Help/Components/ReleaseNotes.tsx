import type { IRelease } from '@/Core/Help/HelpTypes';
import { HelpBlocks } from '@/Modules/Help/Components/HelpBlocks';

export interface IReleaseNotesProps {
  release: IRelease;
  owner: boolean;
}

export const ReleaseNotes = ({ release, owner }: IReleaseNotesProps) => (
  <div className="grid gap-4">
    {release.Sections.filter((section) => section.Kind !== 'actions' || owner).map((section) => (
      <section key={section.Kind} className="grid gap-2">
        <h4 className="m-0 text-label text-primary">{section.Title}</h4>
        <HelpBlocks blocks={section.Body} />
      </section>
    ))}
  </div>
);

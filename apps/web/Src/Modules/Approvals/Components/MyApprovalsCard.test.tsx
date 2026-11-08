import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { approvalFixture, approvalsRoute, newApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { MyApprovalsCard } from '@/Modules/Approvals/Components/MyApprovalsCard';

const OTHER_PROJECT = '0198a000-0000-7000-8000-000000000399';

const mount = (mine: ReturnType<typeof approvalFixture>[], props: { projectId?: string; hideWhenEmpty?: boolean } = {}) => {
  const state = newApprovalsState();
  state.mine = mine;
  startBackend({ routes: [approvalsRoute(state)] });
  return renderWithProviders(<MyApprovalsCard organizationId={ORG_ID} hrefFor={() => '/x'} onOpen={vi.fn()} {...props} />);
};

describe('MyApprovalsCard', () => {
  it('lists every approval of the reader when it has no project', async () => {
    mount([approvalFixture(), approvalFixture({ Id: '0198a000-0000-7000-8000-000000000d09', ProjectId: OTHER_PROJECT })]);
    const list = await screen.findByRole('list', { name: arText('approvals.page.list') });
    expect(within(list).getAllByRole('listitem')).toHaveLength(2);
  });

  it('keeps the approvals of one project under the title of the board', async () => {
    mount([approvalFixture(), approvalFixture({ Id: '0198a000-0000-7000-8000-000000000d09', ProjectId: OTHER_PROJECT })], { projectId: PROJECT_ID });
    const list = await screen.findByRole('list', { name: arText('boards.approvals.title') });
    expect(within(list).getAllByRole('listitem')).toHaveLength(1);
  });

  it('says there is nothing for this project, or shows nothing at all when asked to hide', async () => {
    const shown = mount([approvalFixture({ ProjectId: OTHER_PROJECT })], { projectId: PROJECT_ID });
    expect(await screen.findByText(arText('boards.approvals.empty'))).toBeInTheDocument();
    shown.unmount();

    const hidden = mount([approvalFixture({ ProjectId: OTHER_PROJECT })], { projectId: PROJECT_ID, hideWhenEmpty: true });
    await waitFor(() => {
      expect(hidden.container).toBeEmptyDOMElement();
    });
  });
});

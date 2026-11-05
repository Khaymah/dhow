import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import {
  ENVIRONMENT_NAME,
  environmentFixture,
  environmentsRoute,
  newEnvironmentsState,
  REVIEWER_ID,
  rulesFixture,
  TEAM_ID,
  type IEnvironmentsState,
} from '@/Core/Testing/EnvironmentsBackend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const PASSWORD = 'correct horse battery staple';
const PAGE_URL = `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;
const PATH = `/organizations/${ORG_ID}/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;
const MONA_ID = '0198a000-0000-7000-8000-0000000000b2';

const mount = (role = 'owner', state: IEnvironmentsState = newEnvironmentsState(), url = PAGE_URL) => {
  const backend = startBackend({
    role,
    routes: [environmentsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

const openRules = async () => {
  const main = await findPage(ENVIRONMENT_NAME);
  const region = await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
  return { main, region, wait: await within(region).findByLabelText(Ar['environments.rule.wait']) };
};

const enterFactor = async (view: ReturnType<typeof mount>, region: HTMLElement) => {
  await view.user.click(within(region).getByLabelText(Ar['factor.method.password']));
  await view.user.paste(PASSWORD);
};

const save = (view: ReturnType<typeof mount>, region: HTMLElement) =>
  view.user.click(within(region).getByRole('button', { name: Ar['environments.form.save'] }));

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the rules of an environment for a manager', () => {
  it.each(['owner', 'admin'])('shows a %s the rules as a form with the current values', async (role) => {
    mount(role);
    const { region, wait } = await openRules();
    expect(wait).toHaveValue('10');
    expect(within(region).getByLabelText(Ar['environments.rule.timeout'])).toHaveValue('72');
    expect(within(region).getByLabelText(Ar['environments.refs.pattern'])).toHaveValue('main');
    expect(within(region).getByLabelText(Ar['environments.form.description'])).toHaveValue('Live site');
    expect(within(region).getByRole('checkbox', { name: new RegExp(Ar['environments.rule.supersede']) })).toBeChecked();
    expect(within(region).getByRole('checkbox', { name: new RegExp(Ar['environments.rule.bypass']) })).not.toBeChecked();
    expect(await within(region).findByRole('checkbox', { name: /Hana Samir/ })).toBeChecked();
    expect(within(region).getByRole('button', { name: Ar['environments.form.save'] })).toBeDisabled();
  });

  it('saves only what changed, with the version it read and the second step', async () => {
    const view = mount();
    const { region, wait } = await openRules();
    await view.user.clear(wait);
    await view.user.type(wait, '20');
    await view.user.click(within(region).getByRole('checkbox', { name: new RegExp(Ar['environments.rule.pullRequests']) }));
    await enterFactor(view, region);
    await save(view, region);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'PUT',
      Path: PATH,
      Body: { Version: 3, Rules: { AllowPullRequests: true, WaitMinutes: 20 }, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('environments.saved', { name: ENVIRONMENT_NAME }))).toBeInTheDocument();
    await waitFor(() => {
      expect(within(region).getByLabelText(Ar['environments.rule.wait'])).toHaveValue('20');
    });
    expect(within(region).getByRole('button', { name: Ar['environments.form.save'] })).toBeDisabled();
  });

  it('sends the next version after a save', async () => {
    const view = mount();
    const first = await openRules();
    await view.user.type(within(first.region).getByLabelText(Ar['environments.form.description']), '!');
    await enterFactor(view, first.region);
    await save(view, first.region);
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toEqual({ Version: 3, Description: 'Live site!', CurrentFactor: { Method: 'password', Password: PASSWORD } });

    await waitFor(() => {
      expect(within(first.region).getByLabelText(Ar['environments.form.description'])).toHaveValue('Live site!');
    });
    await view.user.click(within(first.region).getByRole('checkbox', { name: new RegExp(Ar['environments.rule.selfApproval']) }));
    await enterFactor(view, first.region);
    await save(view, first.region);
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(2);
    });
    expect(view.state.writes[1]?.Body).toMatchObject({ Version: 4, Rules: { AllowSelfApproval: true } });
  });

});

describe('the reviewers and patterns of an environment', () => {
  it('changes the reviewers from the lists of members and teams', async () => {
    const view = mount();
    const { region } = await openRules();
    await view.user.click(await within(region).findByRole('checkbox', { name: /Mona Adel/ }));
    await view.user.click(await within(region).findByRole('checkbox', { name: /Backend/ }));
    await enterFactor(view, region);
    await save(view, region);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toMatchObject({
      Version: 3,
      Rules: { Reviewers: { UserIds: [REVIEWER_ID, MONA_ID], TeamIds: [TEAM_ID] } },
    });
  });

  it('adds and removes branch and tag patterns', async () => {
    const view = mount();
    const { region } = await openRules();
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.refs.add'] }));
    const patterns = within(region).getAllByLabelText(Ar['environments.refs.pattern']);
    await view.user.type(patterns[1] ?? region, 'v*');
    await view.user.selectOptions(within(region).getAllByLabelText(Ar['environments.refs.kind'])[1] ?? region, 'tag');
    await view.user.click(within(region).getAllByRole('button', { name: Ar['environments.refs.remove'] })[0] ?? region);
    await enterFactor(view, region);
    await save(view, region);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ Rules: { AllowedRefs: [{ Kind: 'tag', Pattern: 'v*' }] } });
  });

  it('asks again before a save leaves no branch or tag allowed', async () => {
    const view = mount();
    const { region } = await openRules();
    for (const remove of within(region).getAllByRole('button', { name: Ar['environments.refs.remove'] })) {
      await view.user.click(remove);
    }
    await enterFactor(view, region);
    await save(view, region);
    expect(await within(region).findByText(Ar['environments.refs.emptyConfirm'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);

    await save(view, region);
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ Rules: { AllowedRefs: [] } });
  });

  it('opens no secret slide on the address of an environment with an invalid name', async () => {
    const view = mount('owner', newEnvironmentsState(), `/projects/${PROJECT_ID}/environments/Bad_Name?slide=secret-new`);
    await waitFor(() => {
      expect(view.router.state.location.search).not.toHaveProperty('slide');
    });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('refuses a number out of range, an invalid pattern and too many plugins before the server', async () => {
    const view = mount();
    const { region, wait } = await openRules();
    await enterFactor(view, region);
    await view.user.clear(wait);
    await view.user.type(wait, '99999');
    await save(view, region);
    expect(await within(region).findByText(Ar['environments.error.wait'])).toBeInTheDocument();

    await view.user.clear(wait);
    await view.user.type(wait, '5');
    await view.user.type(within(region).getByLabelText(Ar['environments.refs.pattern']), ' x');
    await save(view, region);
    expect(await within(region).findByText(Ar['environments.error.pattern.invalid'])).toBeInTheDocument();

    await view.user.clear(within(region).getByLabelText(Ar['environments.refs.pattern']));
    await view.user.type(within(region).getByLabelText(Ar['environments.refs.pattern']), 'main');
    await view.user.click(within(region).getByLabelText(Ar['environments.rule.plugins']));
    await view.user.paste(Array.from({ length: 51 }, (_, index) => `ghcr.io/acme/p${String(index)}`).join('\n'));
    await save(view, region);
    expect(await within(region).findByText(Ar['environments.error.plugins.many'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

});

describe('saving the rules of an environment fails', () => {
  it('shows the version conflict and loads the latest version on request', async () => {
    const state = newEnvironmentsState();
    state.failWrite = backendFailure('ENVIRONMENT_VERSION_CONFLICT', 'تغيّرت البيئة منذ قرأتها.', 409);
    const view = mount('owner', state);
    const { region, wait } = await openRules();
    await view.user.clear(wait);
    await view.user.type(wait, '30');
    await enterFactor(view, region);
    await save(view, region);

    expect(await within(region).findByText('تغيّرت البيئة منذ قرأتها.')).toBeInTheDocument();
    state.failWrite = null;
    state.environments = [environmentFixture({ Version: 4, RuleVersion: 4, Rules: rulesFixture({ WaitMinutes: 45 }) })];
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.conflict.reload'] }));
    await waitFor(() => {
      expect(within(region).getByLabelText(Ar['environments.rule.wait'])).toHaveValue('45');
    });
    expect(within(region).queryByText('تغيّرت البيئة منذ قرأتها.')).not.toBeInTheDocument();
  });

  it('shows the reviewer that is not eligible', async () => {
    const state = newEnvironmentsState();
    state.failWrite = new Response(
      JSON.stringify({
        Success: false,
        Error: {
          Code: 'REVIEWER_NOT_ELIGIBLE',
          Message: 'المراجع غير مؤهل.',
          Details: [{ Field: 'Rules.Reviewers.UserIds', Message: 'دور المراجع على المشروع أقل من مطوّر.' }],
        },
      }),
      { status: 422, headers: { 'Content-Type': 'application/json' } },
    );
    const view = mount('owner', state);
    const { region } = await openRules();
    await view.user.click(await within(region).findByRole('checkbox', { name: /Mona Adel/ }));
    await enterFactor(view, region);
    await save(view, region);
    const alert = await within(region).findByRole('alert');
    expect(alert).toHaveTextContent('المراجع غير مؤهل.');
    expect(alert).toHaveTextContent('دور المراجع على المشروع أقل من مطوّر.');
  });

  it('shows the server message and keeps the form when the second step is wrong', async () => {
    const state = newEnvironmentsState();
    state.failWrite = backendFailure('INVALID_CURRENT_FACTOR', 'تعذّر التحقق من خطوتك الثانية.', 403);
    const view = mount('owner', state);
    const { region, wait } = await openRules();
    await view.user.clear(wait);
    await view.user.type(wait, '11');
    await enterFactor(view, region);
    await save(view, region);
    expect(await within(region).findByText('تعذّر التحقق من خطوتك الثانية.')).toBeInTheDocument();
    expect(within(region).getByLabelText(Ar['environments.rule.wait'])).toHaveValue('11');
  });

  it('asks for the second step before it saves', async () => {
    const view = mount();
    const { region, wait } = await openRules();
    await view.user.clear(wait);
    await view.user.type(wait, '12');
    await save(view, region);
    expect(await within(region).findByText(Ar['factor.error.required'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('does not offer the form when the project is archived', async () => {
    const state = newEnvironmentsState();
    startBackend({
      role: 'owner',
      routes: [environmentsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: 'owner', State: 'archived' })) })],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [PAGE_URL] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
    expect(await within(region).findByText(Ar['environments.refs.title'])).toBeInTheDocument();
    expect(within(region).queryByRole('button', { name: Ar['environments.form.save'] })).not.toBeInTheDocument();
  });
});

describe('the rules of an environment for everyone else', () => {
  it.each(['maintainer', 'developer', 'viewer'])('shows a %s the rules to read, with no form', async (role) => {
    const state = newEnvironmentsState();
    state.environments = [
      environmentFixture({
        Rules: rulesFixture({
          AllowedRefs: [
            { Kind: 'branch', Pattern: 'main' },
            { Kind: 'tag', Pattern: 'v*' },
          ],
          Reviewers: { UserIds: [], TeamIds: [], UserCount: 2, TeamCount: 1, IdsShown: false },
        }),
      }),
    ];
    const view = mount(role, state);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
    expect(await within(region).findByText(Ar['environments.rules.readOnly'])).toBeInTheDocument();
    expect(within(region).getByText('v*')).toBeInTheDocument();
    expect(within(region).getByText(arText('environments.summary.reviewers', { users: 2, teams: 1 }))).toBeInTheDocument();
    expect(within(region).getByText(arText('environments.summary.minutes', { count: 10 }))).toBeInTheDocument();
    expect(within(region).queryByRole('textbox')).not.toBeInTheDocument();
    expect(within(region).queryByRole('button', { name: Ar['environments.form.save'] })).not.toBeInTheDocument();
    expect(view.calls.some((call) => call.url.includes('/members') || call.url.includes('/teams'))).toBe(false);
  });

  it('shows a manager whose session hides the reviewer ids the counts and keeps the form for the rest', async () => {
    const state = newEnvironmentsState();
    state.environments = [
      environmentFixture({ Rules: rulesFixture({ Reviewers: { UserIds: [], TeamIds: [], UserCount: 1, TeamCount: 0, IdsShown: false } }) }),
    ];
    const view = mount('owner', state);
    const { region, wait } = await openRules();
    expect(within(region).getByText(Ar['environments.reviewers.hidden'])).toBeInTheDocument();
    expect(within(region).queryByRole('checkbox', { name: /Hana Samir/ })).not.toBeInTheDocument();
    await view.user.clear(wait);
    await view.user.type(wait, '3');
    await enterFactor(view, region);
    await save(view, region);
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toEqual({ Version: 3, Rules: { WaitMinutes: 3 }, CurrentFactor: { Method: 'password', Password: PASSWORD } });
  });
});

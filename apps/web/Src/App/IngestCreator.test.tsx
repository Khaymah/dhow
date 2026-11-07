import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { ingestTokenFixture, newIngestState, TOKEN_ID } from '@/Core/Testing/IngestBackend';
import { API, mount, PASSWORD, PROJECT_NAME } from '@/Core/Testing/IngestHarness';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('a token whose creator is no longer allowed', () => {
  it('says so on the row of the token', async () => {
    const state = newIngestState();
    state.tokens = [ingestTokenFixture({ CreatorAllowed: false })];
    mount(undefined, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(/لم يعد .* عضوًا يملك صلاحية إدارة الرفع/)).toBeInTheDocument();
  });

  it('says nothing for a token whose creator is allowed', async () => {
    mount();
    const main = await findPage(PROJECT_NAME);
    await within(main).findByRole('list', { name: Ar['ingest.tokens.title'] });
    expect(within(main).queryByText(/لم يعد .* عضوًا يملك صلاحية إدارة الرفع/)).not.toBeInTheDocument();
  });
});

describe('a token that reaches a protected environment nobody confirmed', () => {
  const unconfirmed = () => {
    const state = newIngestState();
    state.tokens = [ingestTokenFixture({ UnacknowledgedReach: ['production'] })];
    return state;
  };

  it('names the environment and offers the confirmation to a maintainer', async () => {
    mount(undefined, 'maintainer', unconfirmed());
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['ingest.token.unconfirmed'])).toBeInTheDocument();
    expect(within(main).getByRole('button', { name: arText('ingest.ack.aria', { name: 'github-main' }) })).toBeInTheDocument();
  });

  it('sends the confirmation with the second step and clears the warning', async () => {
    const view = mount(undefined, 'owner', unconfirmed());
    const main = await findPage(PROJECT_NAME);
    await view.user.click(await within(main).findByRole('button', { name: arText('ingest.ack.aria', { name: 'github-main' }) }));
    expect(await within(main).findByText(arText('ingest.ack.question', { name: 'github-main', environments: 'production' }))).toBeInTheDocument();
    expect(view.state.writes).toEqual([]);

    await view.user.click(await within(main).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(main).getByRole('button', { name: Ar['ingest.ack.confirm'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/ingest-tokens/${TOKEN_ID}/acknowledge-reach`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('ingest.ack.done', { name: 'github-main' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(within(main).queryByText(Ar['ingest.token.unconfirmed'])).not.toBeInTheDocument();
    });
  });
});

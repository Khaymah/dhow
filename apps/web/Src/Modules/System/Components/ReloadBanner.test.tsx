import { screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { envelope, META_BODY, notFound, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { ReloadBanner } from '@/Modules/System/Components/ReloadBanner';
import { META_QUERY_KEY } from '@/Modules/System/Hooks/useMeta';
import { BUILD_CHECK_INTERVAL_MS, sameBuild } from '@/Modules/System/Hooks/useServerBuildChanged';

const OLD_BUILD = { Version: 'v1.4.0', Commit: '9f3c2ab', BuiltAt: '2026-11-06T09:30:00Z' };
const NEW_BUILD = { Version: 'v1.5.0', Commit: '1a2b3c4', BuiltAt: '2026-11-07T10:00:00Z' };

const metaWith = (build?: Record<string, string>) => ({ ...META_BODY, Data: { ...META_BODY.Data, ...(build === undefined ? {} : { ServerBuild: build }) } });

const serve = (initial?: Record<string, string>) => {
  let current = metaWith(initial);
  const stub = stubFetch((request) => (new URL(request.url).pathname.endsWith('/meta') ? envelope(current) : notFound()));
  const metaCalls = () => stub.calls.filter((call) => new URL(call.url).pathname.endsWith('/meta')).length;
  return {
    ...stub,
    metaCalls,
    deploy: (build?: Record<string, string>) => {
      current = metaWith(build);
    },
  };
};

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('ReloadBanner', () => {
  it('stays hidden while the server build is the one the page loaded with', async () => {
    const server = serve(OLD_BUILD);
    const { client } = renderWithProviders(<ReloadBanner />);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(1);
    });
    await client.refetchQueries({ queryKey: META_QUERY_KEY });
    expect(server.metaCalls()).toBe(2);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('asks for a reload when the server build changed, and reloads on the button', async () => {
    const server = serve(OLD_BUILD);
    const reload = vi.fn();
    vi.spyOn(window, 'location', 'get').mockReturnValue({ reload } as unknown as Location);
    const { client, user } = renderWithProviders(<ReloadBanner />);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(1);
    });
    server.deploy(NEW_BUILD);
    await client.refetchQueries({ queryKey: META_QUERY_KEY });
    const banner = await screen.findByRole('status');
    expect(banner).toHaveTextContent(Ar['about.reload.title']);
    expect(banner).toHaveTextContent(Ar['about.reload.body']);
    await user.click(screen.getByRole('button', { name: Ar['about.reload.action'] }));
    expect(reload).toHaveBeenCalledOnce();
  });

  it('notices a new commit even when the version label did not change', async () => {
    const server = serve(OLD_BUILD);
    const { client } = renderWithProviders(<ReloadBanner />);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(1);
    });
    server.deploy({ ...OLD_BUILD, Commit: 'ffffff0' });
    await client.refetchQueries({ queryKey: META_QUERY_KEY });
    expect(await screen.findByRole('status')).toBeInTheDocument();
  });

  it('never shows the bar when the server does not report a build', async () => {
    const server = serve();
    const { client } = renderWithProviders(<ReloadBanner />);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(1);
    });
    server.deploy(NEW_BUILD);
    await client.refetchQueries({ queryKey: META_QUERY_KEY });
    server.deploy();
    await client.refetchQueries({ queryKey: META_QUERY_KEY });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('checks again only after the interval, so a quiet page makes one request every few minutes', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const server = serve(OLD_BUILD);
    renderWithProviders(<ReloadBanner />);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(1);
    });
    await vi.advanceTimersByTimeAsync(BUILD_CHECK_INTERVAL_MS - 1000);
    expect(server.metaCalls()).toBe(1);
    await vi.advanceTimersByTimeAsync(2000);
    await waitFor(() => {
      expect(server.metaCalls()).toBe(2);
    });
  });

  it('compares the version, the commit and the build time', () => {
    expect(sameBuild(OLD_BUILD, { ...OLD_BUILD })).toBe(true);
    expect(sameBuild(OLD_BUILD, NEW_BUILD)).toBe(false);
    expect(sameBuild({ Version: 'dev', Commit: '' }, { Version: 'dev', Commit: '' })).toBe(true);
    expect(sameBuild({ Version: 'dev', Commit: '' }, { Version: 'dev', Commit: '', BuiltAt: NEW_BUILD.BuiltAt })).toBe(false);
  });

  it('polls no faster than once a minute', () => {
    expect(BUILD_CHECK_INTERVAL_MS).toBeGreaterThanOrEqual(60_000);
  });
});

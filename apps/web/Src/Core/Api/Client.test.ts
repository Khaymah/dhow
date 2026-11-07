import { describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/Core/Api/ApiError';
import { ApiClient, request } from '@/Core/Api/Client';
import { envelope, META_BODY, stubFetch } from '@/Core/Testing/Fetch';

const getMeta = () => request(() => ApiClient.GET('/meta'));

describe('API client', () => {
  it('calls the versioned API on the same origin', async () => {
    const { calls } = stubFetch(() => envelope(META_BODY));
    await getMeta();
    const url = new URL(calls[0]?.url ?? '');
    expect(url.origin).toBe(window.location.origin);
    expect(url.pathname).toBe('/api/v1/meta');
  });

  it('sends Accept-Language from the document language', async () => {
    const { calls } = stubFetch(() => envelope(META_BODY));
    document.documentElement.lang = 'ar';
    await getMeta();
    document.documentElement.lang = 'en';
    await getMeta();
    expect(calls.map((call) => call.headers.get('Accept-Language'))).toEqual(['ar', 'en']);
  });

  it('unwraps the envelope and returns only Data', async () => {
    stubFetch(() => envelope(META_BODY));
    await expect(getMeta()).resolves.toEqual(META_BODY.Data);
  });

  it('turns an error envelope into an ApiError carrying Code and the server message', async () => {
    stubFetch(() => envelope({ Success: false, Error: { Code: 'SERVICE_UNAVAILABLE', Message: 'غير جاهز' } }, 503));
    const failure = await getMeta().catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect(failure).toMatchObject({ Code: 'SERVICE_UNAVAILABLE', Message: 'غير جاهز', Status: 503 });
  });

  it('keeps Fix and DocsPath from the error envelope and defaults them to empty', async () => {
    stubFetch(() =>
      envelope(
        { Success: false, Error: { Code: 'CSRF_FAILED', Message: 'm', Fix: 'Reload the page.', DocsPath: 'install/troubleshooting.md' } },
        403,
      ),
    );
    await expect(getMeta().catch((error: unknown) => error)).resolves.toMatchObject({
      Fix: 'Reload the page.',
      DocsPath: 'install/troubleshooting.md',
    });
    stubFetch(() => envelope({ Success: false, Error: { Code: 'X', Message: 'm', Fix: 7, DocsPath: null } }, 400));
    await expect(getMeta().catch((error: unknown) => error)).resolves.toMatchObject({ Fix: '', DocsPath: '' });
  });

  it('records the language the server answered in', async () => {
    stubFetch(() =>
      new Response(JSON.stringify({ Success: false, Error: { Code: 'X', Message: 'm' } }), {
        status: 400,
        headers: { 'Content-Type': 'application/json', 'Content-Language': 'EN' },
      }),
    );
    await expect(getMeta().catch((error: unknown) => error)).resolves.toMatchObject({ Language: 'en' });
  });

  it('reports an unreachable server as NETWORK', async () => {
    stubFetch(() => {
      throw new TypeError('Failed to fetch');
    });
    await expect(getMeta()).rejects.toMatchObject({ Code: 'NETWORK', Status: 0 });
  });

  it('reports a reply that is not JSON as INVALID_RESPONSE', async () => {
    stubFetch(() => new Response('<html></html>', { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await expect(getMeta()).rejects.toMatchObject({ Code: 'INVALID_RESPONSE' });
  });

  it('reports an error reply without an envelope as INVALID_RESPONSE', async () => {
    stubFetch(() => new Response('bad gateway', { status: 502 }));
    await expect(getMeta()).rejects.toMatchObject({ Code: 'INVALID_RESPONSE', Status: 502 });
  });

  it('can keep the Data of a failed reply when the caller asks for it', async () => {
    const body = { Success: false, Error: { Code: 'SERVICE_UNAVAILABLE', Message: 'x' }, Data: { Status: 'unavailable', Checks: [] } };
    stubFetch(() => envelope(body, 503));
    const data = await request(() => ApiClient.GET('/health/ready'), { acceptFailureData: true });
    expect(data).toEqual(body.Data);
    await expect(request(() => ApiClient.GET('/health/ready'))).rejects.toBeInstanceOf(ApiError);
  });

  it('does not touch the network while nothing is called', () => {
    const { stub } = stubFetch(() => envelope(META_BODY));
    expect(vi.mocked(stub)).not.toHaveBeenCalled();
  });
});

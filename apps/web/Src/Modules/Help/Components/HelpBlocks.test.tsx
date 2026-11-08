import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { IRelease, TBlock } from '@/Core/Help/HelpTypes';
import { I18nProvider } from '@/I18n/I18nProvider';
import { HelpBlocks } from '@/Modules/Help/Components/HelpBlocks';
import { ReleaseNotes } from '@/Modules/Help/Components/ReleaseNotes';

const show = (blocks: TBlock[]) =>
  render(
    <I18nProvider>
      <HelpBlocks blocks={blocks} />
    </I18nProvider>,
  );

describe('HelpBlocks', () => {
  it('renders text as text, never as markup', () => {
    show([
      { Type: 'paragraph', Inline: [{ Type: 'text', Text: '<img src=x onerror=alert(1)> <script>alert(1)</script>' }] },
      { Type: 'code', Text: '<b>not bold</b>' },
    ]);
    expect(document.querySelector('img')).toBeNull();
    expect(document.querySelector('script')).toBeNull();
    expect(document.querySelector('b')).toBeNull();
    expect(screen.getByText('<b>not bold</b>')).toBeInTheDocument();
  });

  it('renders an allowed documentation link and an allowed repository link', () => {
    show([
      {
        Type: 'paragraph',
        Inline: [
          { Type: 'link', Kind: 'doc', Text: 'jobs', Doc: 'guides/jobs.md' },
          { Type: 'link', Kind: 'external', Text: 'repo', Href: 'https://github.com/Khaymah/dhow/issues' },
        ],
      },
    ]);
    expect(screen.getByRole('link', { name: /jobs/ })).toHaveAttribute('href', 'https://github.com/Khaymah/dhow/blob/main/docs/guides/jobs.md');
    expect(screen.getByRole('link', { name: /repo/ })).toHaveAttribute('href', 'https://github.com/Khaymah/dhow/issues');
  });

  it.each([
    ['a javascript link', { Type: 'link', Kind: 'external', Text: 'x', Href: 'javascript:alert(1)' }],
    ['a data link', { Type: 'link', Kind: 'external', Text: 'x', Href: 'data:text/html,hi' }],
    ['another host', { Type: 'link', Kind: 'external', Text: 'x', Href: 'https://evil.example/Khaymah/dhow' }],
    ['a host that starts like ours', { Type: 'link', Kind: 'external', Text: 'x', Href: 'https://github.com/Khaymah/dhow.evil.example' }],
    ['a bad docs path', { Type: 'link', Kind: 'doc', Text: 'x', Doc: '../../etc/passwd.md' }],
  ] as const)('shows %s as plain text, not as a link', (_name, node) => {
    show([{ Type: 'paragraph', Inline: [node] }]);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('x')).toBeInTheDocument();
  });

  it('renders ordered and unordered lists', () => {
    show([
      { Type: 'list', Ordered: true, Items: [[{ Type: 'text', Text: 'one' }]] },
      { Type: 'list', Ordered: false, Items: [[{ Type: 'text', Text: 'two' }]] },
    ]);
    expect(document.querySelector('ol li')).toHaveTextContent('one');
    expect(document.querySelector('ul li')).toHaveTextContent('two');
  });
});

const RELEASE: IRelease = {
  Version: '1.0.0',
  Title: 'Dhow 1.0.0',
  Sections: [
    { Kind: 'highlights', Title: 'Highlights', Body: [{ Type: 'paragraph', Inline: [{ Type: 'text', Text: 'Fast' }] }] },
    { Kind: 'actions', Title: 'Actions required', Body: [{ Type: 'paragraph', Inline: [{ Type: 'text', Text: 'Rotate keys' }] }] },
  ],
};

describe('ReleaseNotes', () => {
  it('hides the actions section from everyone but the server owner', () => {
    const { unmount } = render(
      <I18nProvider>
        <ReleaseNotes release={RELEASE} owner={false} />
      </I18nProvider>,
    );
    expect(screen.getByText('Highlights')).toBeInTheDocument();
    expect(screen.queryByText('Actions required')).toBeNull();
    unmount();
    render(
      <I18nProvider>
        <ReleaseNotes release={RELEASE} owner />
      </I18nProvider>,
    );
    expect(screen.getByText('Actions required')).toBeInTheDocument();
    expect(screen.getByText('Rotate keys')).toBeInTheDocument();
  });
});

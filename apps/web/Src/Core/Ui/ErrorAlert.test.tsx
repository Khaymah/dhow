import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';

import { ApiError } from '@/Core/Api/ApiError';
import { renderWithProviders } from '@/Core/Testing/Render';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { STORAGE_KEY } from '@/I18n/Locale';

const withFix = new ApiError(
  {
    Code: 'CSRF_FAILED',
    Message: 'The request could not be verified.',
    Fix: 'Reload the page and try again.',
    DocsPath: 'install/troubleshooting.md#finding-the-setup-link',
  },
  403,
);

beforeEach(() => {
  window.localStorage.setItem(STORAGE_KEY, 'en');
});

describe('ErrorAlert', () => {
  it('shows the fix under the message inside an alert that describes itself with it', () => {
    renderWithProviders(<ErrorAlert error={withFix} />);
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('The request could not be verified.');
    const fix = screen.getByText('Reload the page and try again.');
    expect(alert).toContainElement(fix);
    expect(alert.getAttribute('aria-describedby')).toBe(fix.id);
    expect(fix).toHaveTextContent('Fix:');
  });

  it('links to the docs page of the UI language and ties the link to the fix', () => {
    renderWithProviders(<ErrorAlert error={withFix} />);
    const link = screen.getByRole('link', { name: 'Learn more' });
    expect(link).toHaveAttribute(
      'href',
      'https://github.com/Khaymah/dhow/blob/main/docs/en/install/troubleshooting.md#finding-the-setup-link',
    );
    expect(link).toHaveAttribute('rel', 'noreferrer noopener');
    expect(link.getAttribute('aria-describedby')).toBe(screen.getByText('Reload the page and try again.').id);
  });

  it('uses the Arabic docs folder when the UI is Arabic', () => {
    window.localStorage.setItem(STORAGE_KEY, 'ar');
    renderWithProviders(<ErrorAlert error={withFix} />);
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      expect.stringContaining('/blob/main/docs/install/troubleshooting.md#finding-the-setup-link'),
    );
  });

  it('shows only the message for an error without a fix or a docs path', () => {
    renderWithProviders(<ErrorAlert error={new ApiError({ Code: 'METHOD_NOT_ALLOWED', Message: 'Not allowed.' }, 405)} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Not allowed.');
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByRole('alert')).not.toHaveAttribute('aria-describedby');
  });

  it('drops a docs path that is not a plain relative markdown path', () => {
    const hostile = new ApiError(
      { Code: 'X', Message: 'Bad.', Fix: 'Do this.', DocsPath: 'https://evil.example/page.md' },
      400,
    );
    renderWithProviders(<ErrorAlert error={hostile} />);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('Do this.')).toBeInTheDocument();
  });

  it('falls back to the generic text for a value that is not an ApiError', () => {
    renderWithProviders(<ErrorAlert error={new Error('boom')} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong. Try again.');
    expect(screen.queryByText('boom')).toBeNull();
  });
});

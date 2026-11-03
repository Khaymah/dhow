import { render, screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/Core/Testing/Render';
import { Avatar, initialOf } from '@/Core/Ui/Avatar';
import { AuthPage } from '@/Core/Ui/AuthPage';
import { Field } from '@/Core/Ui/Field';
import { Pager, pageCount } from '@/Core/Ui/Pager';
import { PasswordInput } from '@/Core/Ui/PasswordInput';
import { StrengthMeter } from '@/Core/Ui/StrengthMeter';
import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';
import { isolate } from '@/I18n/Translate';

const Typing = () => {
  const [Value, setValue] = useState('');
  return (
    <>
      <Field label="pw">
        <PasswordInput
          value={Value}
          onChange={(event) => {
            setValue(event.target.value);
          }}
        />
      </Field>
      <StrengthMeter password={Value} />
    </>
  );
};

describe('PasswordInput', () => {
  it('hides the password until the person asks to see it', async () => {
    const { user } = renderWithProviders(<Typing />);
    const input = screen.getByLabelText('pw');
    expect(input).toHaveAttribute('type', 'password');
    await user.click(screen.getByRole('button', { name: Ar['password.show'] }));
    expect(input).toHaveAttribute('type', 'text');
    await user.click(screen.getByRole('button', { name: Ar['password.hide'] }));
    expect(input).toHaveAttribute('type', 'password');
  });
});

describe('StrengthMeter', () => {
  it('exposes the score as a meter and says it in words', async () => {
    const { user } = renderWithProviders(<Typing />);
    const meter = screen.getByRole('meter', { name: Ar['password.strength.label'] });
    expect(meter).toHaveAttribute('aria-valuenow', '0');
    expect(screen.getByText(Ar['password.strength.empty'])).toBeInTheDocument();
    await user.type(screen.getByLabelText('pw'), 'short');
    expect(meter).toHaveAttribute('aria-valuenow', '1');
    expect(meter).toHaveAttribute('aria-valuetext', Ar['password.strength.weak']);
    await user.type(screen.getByLabelText('pw'), ' and now a much longer phrase 99');
    expect(meter).toHaveAttribute('aria-valuenow', '4');
    expect(screen.getByText(Ar['password.strength.strong'])).toBeInTheDocument();
  });
});

describe('Pager', () => {
  it('counts pages and renders nothing for a single page', () => {
    expect(pageCount(0, 20)).toBe(1);
    expect(pageCount(41, 20)).toBe(3);
    const { container } = renderWithProviders(<Pager page={1} limit={20} total={20} onPage={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('moves between pages and disables the ends', async () => {
    const onPage = vi.fn();
    const { user } = renderWithProviders(<Pager page={1} limit={20} total={41} onPage={onPage} />);
    expect(screen.getByText(Ar['pager.status'].replace('{page}', isolate('1')).replace('{pages}', isolate('3')))).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['pager.previous'] })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: Ar['pager.next'] }));
    expect(onPage).toHaveBeenCalledWith(2);
  });

  it('disables next on the last page', () => {
    renderWithProviders(<Pager page={3} limit={20} total={41} onPage={vi.fn()} />);
    expect(screen.getByRole('button', { name: Ar['pager.next'] })).toBeDisabled();
    expect(screen.getByRole('button', { name: Ar['pager.previous'] })).toBeEnabled();
  });
});

describe('Avatar', () => {
  it('shows the first letter of the name and hides it from screen readers', () => {
    expect(initialOf('  sabry')).toBe('S');
    expect(initialOf('')).toBe('');
    expect(initialOf('صبري')).toBe('ص');
    const { container } = render(<Avatar name="Mona" />);
    expect(container.firstElementChild).toHaveAttribute('aria-hidden', 'true');
    expect(container).toHaveTextContent('M');
  });
});

describe('AuthPage', () => {
  it('has one main landmark and one heading, and switches the language', async () => {
    const { user } = renderWithProviders(
      <AuthPage title="العنوان" lead="الشرح" footer={<a href="/x">رابط</a>}>
        <p>المحتوى</p>
      </AuthPage>,
    );
    expect(screen.getAllByRole('main')).toHaveLength(1);
    expect(screen.getByRole('heading', { level: 1, name: 'العنوان' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['shell.language.switch'].replace('{language}', isolate(En['language.en'])) }));
    expect(document.documentElement.lang).toBe('en');
    expect(document.documentElement.dir).toBe('ltr');
  });
});

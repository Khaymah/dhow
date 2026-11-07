import { renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';

import { ApiError } from '@/Core/Api/ApiError';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { I18nProvider } from '@/I18n/I18nProvider';

const wrapper = ({ children }: { children: ReactNode }) => <I18nProvider>{children}</I18nProvider>;

describe('useApiErrorText', () => {
  it('adds the fix after the message so plain-text error spots show it too', () => {
    const { result } = renderHook(() => useApiErrorText(), { wrapper });
    const withFix = new ApiError({ Code: 'X', Message: 'Name taken.', Fix: 'Choose another name.' }, 409);
    const without = new ApiError({ Code: 'X', Message: 'Name taken.' }, 409);
    expect(result.current(withFix)).toBe('Name taken. Choose another name.');
    expect(result.current(without)).toBe('Name taken.');
  });
});

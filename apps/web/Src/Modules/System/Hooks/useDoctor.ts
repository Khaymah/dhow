import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import { getDoctor } from '@/Modules/System/Api/SystemApi';
import type { TDoctorReport } from '@/Modules/System/Types/SystemTypes';

export const DOCTOR_QUERY_KEY = ['system', 'doctor'] as const;

export const useDoctor = (): UseQueryResult<TDoctorReport, ApiError> =>
  useQuery<TDoctorReport, ApiError>({ queryKey: DOCTOR_QUERY_KEY, queryFn: getDoctor });

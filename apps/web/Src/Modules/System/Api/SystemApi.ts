import { ApiClient, request } from '@/Core/Api/Client';
import type { TDoctorReport, THealthStatus, TMeta } from '@/Modules/System/Types/SystemTypes';

export const getMeta = (): Promise<TMeta> => request(() => ApiClient.GET('/meta'));

export const getReadiness = (): Promise<THealthStatus> =>
  request(() => ApiClient.GET('/health/ready'), { acceptFailureData: true });

export const getDoctor = (): Promise<TDoctorReport> => request(() => ApiClient.GET('/system/doctor'));

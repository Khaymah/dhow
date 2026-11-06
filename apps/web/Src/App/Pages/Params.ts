const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export const isUuid = (value: unknown): value is string => typeof value === 'string' && UUID.test(value);

const ENVIRONMENT_NAME = /^[a-z0-9][a-z0-9_-]{0,63}$/;

export const isEnvironmentName = (value: unknown): value is string => typeof value === 'string' && ENVIRONMENT_NAME.test(value);

export const isTargetName = (value: unknown): value is string => isEnvironmentName(value);

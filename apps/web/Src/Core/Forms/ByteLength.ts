const ENCODER = new TextEncoder();

export const byteLength = (value: string): number => ENCODER.encode(value).length;

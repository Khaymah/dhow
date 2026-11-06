import { createHash } from 'node:crypto';
import { gzipSync } from 'node:zlib';

import { stack } from './Stack';

const BLOCK = 512;

const octal = (value: number, length: number): string => `${value.toString(8).padStart(length - 1, '0')}\0`;

const header = (name: string, size: number): Buffer => {
  const block = Buffer.alloc(BLOCK);
  block.write(name, 0, 100, 'utf8');
  block.write(octal(0o644, 8), 100, 8, 'ascii');
  block.write(octal(0, 8), 108, 8, 'ascii');
  block.write(octal(0, 8), 116, 8, 'ascii');
  block.write(octal(size, 12), 124, 12, 'ascii');
  block.write(octal(0, 12), 136, 12, 'ascii');
  block.write('        ', 148, 8, 'ascii');
  block.write('0', 156, 1, 'ascii');
  block.write('ustar\0', 257, 6, 'ascii');
  block.write('00', 263, 2, 'ascii');
  const sum = block.reduce((total, byte) => total + byte, 0);
  block.write(`${sum.toString(8).padStart(6, '0')}\0 `, 148, 8, 'ascii');
  return block;
};

export const tarGz = (files: Record<string, string>): Buffer => {
  const parts: Buffer[] = [];
  for (const name of Object.keys(files).sort()) {
    const data = Buffer.from(files[name] ?? '', 'utf8');
    parts.push(header(name, data.length), data, Buffer.alloc((BLOCK - (data.length % BLOCK)) % BLOCK));
  }
  parts.push(Buffer.alloc(BLOCK * 2));
  return gzipSync(Buffer.concat(parts));
};

export const digestOf = (body: Buffer): string => createHash('sha256').update(body).digest('hex');

export interface IUploadResult {
  Status: number;
  Code: string;
  Digest: string;
}

export const upload = async (token: string, body: Buffer, name: string, ref: string, sha: string): Promise<IUploadResult> => {
  const digest = digestOf(body);
  const response = await fetch(`${stack().Url}/ingest/v1/artifacts/${digest}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/gzip',
      'Dhow-Ingest-Name': name,
      'Dhow-Ingest-Ref': ref,
      'Dhow-Ingest-Sha': sha,
      'Dhow-Ingest-Trigger': 'false',
    },
    body: new Uint8Array(body),
  });
  const parsed: unknown = await response.json();
  const error = typeof parsed === 'object' && parsed !== null && 'Error' in parsed ? (parsed as { Error?: { Code?: string } }).Error : undefined;
  return { Status: response.status, Code: error?.Code ?? '', Digest: digest };
};

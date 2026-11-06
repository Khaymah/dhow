import { createHash, generateKeyPairSync } from 'node:crypto';

const ED25519_SPKI_PREFIX_BYTES = 12;

const lengthPrefixed = (bytes: Buffer): Buffer => {
  const size = Buffer.alloc(4);
  size.writeUInt32BE(bytes.length);
  return Buffer.concat([size, bytes]);
};

export interface IHostKey {
  Line: string;
  Fingerprint: string;
}

export const newHostKey = (host: string): IHostKey => {
  const { publicKey } = generateKeyPairSync('ed25519');
  const raw = publicKey.export({ type: 'spki', format: 'der' }).subarray(ED25519_SPKI_PREFIX_BYTES);
  const blob = Buffer.concat([lengthPrefixed(Buffer.from('ssh-ed25519')), lengthPrefixed(raw)]);
  const fingerprint = `SHA256:${createHash('sha256').update(blob).digest('base64').replace(/=+$/, '')}`;
  return { Line: `${host} ssh-ed25519 ${blob.toString('base64')}`, Fingerprint: fingerprint };
};

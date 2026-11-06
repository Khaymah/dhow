const DIGEST_SHORT = 12;
const SHA_SHORT = 8;
const REF_PREFIXES = ['refs/heads/', 'refs/tags/'] as const;

export const shortDigest = (digest: string): string => digest.slice(0, DIGEST_SHORT);

export const shortCommit = (sha: string): string => sha.slice(0, SHA_SHORT);

export const shortRefName = (ref: string): string => {
  const prefix = REF_PREFIXES.find((item) => ref.startsWith(item));
  return prefix === undefined ? ref : ref.slice(prefix.length);
};

export const provenanceEntries = (provenance: Readonly<Record<string, string>>): [string, string][] =>
  Object.entries(provenance).sort(([left], [right]) => left.localeCompare(right));

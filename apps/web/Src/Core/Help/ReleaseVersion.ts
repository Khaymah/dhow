const VERSION = /^(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/;

const partsOf = (version: string): number[] | null => {
  const match = VERSION.exec(version);
  if (!match) return null;
  const candidate = match[4] === undefined ? Number.POSITIVE_INFINITY : Number(match[4]);
  return [Number(match[1]), Number(match[2]), Number(match[3]), candidate];
};

export const isNewerRelease = (latest: string, seen: string | null | undefined): boolean => {
  if (seen === null || seen === undefined) return true;
  const [a, b] = [partsOf(latest), partsOf(seen)];
  if (a === null || b === null) return latest !== seen;
  for (let index = 0; index < a.length; index += 1) {
    const [x, y] = [a[index] ?? 0, b[index] ?? 0];
    if (x !== y) return x > y;
  }
  return false;
};

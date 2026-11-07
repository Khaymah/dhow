export const KB = 1024;

export const chunkName = (ref) => {
  const file = ref.slice(ref.lastIndexOf('/') + 1).replace(/\.js$/, '');
  return file.replace(/-[\w-]{8}$/, '');
};

export const lazyBudgetKb = (name, config) => {
  const match = config.LazyOverrides.find((rule) => new RegExp(rule.Pattern).test(name));
  return match === undefined ? config.LazyJsKb : match.Kb;
};

const lowered = (flag, configured, requested) => {
  if (requested === undefined) return configured;
  if (requested > configured) {
    throw new Error(`--${flag}=${requested} is above the configured ${configured} KB: overrides can only lower a budget`);
  }
  return requested;
};

export const withOverrides = (config, overrides) => {
  const lazy = lowered('lazy-kb', config.LazyJsKb, overrides.LazyJsKb);
  return {
    ...config,
    InitialJsKb: lowered('initial-js-kb', config.InitialJsKb, overrides.InitialJsKb),
    InitialCssKb: lowered('initial-css-kb', config.InitialCssKb, overrides.InitialCssKb),
    LazyJsKb: lazy,
    LazyOverrides: config.LazyOverrides.map((rule) => ({ ...rule, Kb: Math.min(rule.Kb, lazy) })),
  };
};

export const overrideMatches = (names, config) =>
  config.LazyOverrides.map((rule) => ({
    Pattern: rule.Pattern,
    Kb: rule.Kb,
    Matches: names.filter((name) => new RegExp(rule.Pattern).test(name)).length,
  }));

export const totalBytes = (sizes) => sizes.reduce((sum, [, bytes]) => sum + bytes, 0);

export const evaluate = ({ InitialJs, InitialCss, Lazy }, config) => {
  const initialJs = totalBytes(InitialJs);
  const initialCss = totalBytes(InitialCss);
  const lazy = Lazy.map(([ref, bytes]) => {
    const name = chunkName(ref);
    const budget = lazyBudgetKb(name, config) * KB;
    return { Ref: ref, Name: name, Bytes: bytes, Budget: budget, Over: bytes > budget };
  }).sort((a, b) => b.Bytes - a.Bytes);
  return {
    Initial: [
      { Kind: 'js', Bytes: initialJs, Budget: config.InitialJsKb * KB, Over: initialJs > config.InitialJsKb * KB },
      { Kind: 'css', Bytes: initialCss, Budget: config.InitialCssKb * KB, Over: initialCss > config.InitialCssKb * KB },
    ],
    Lazy: lazy,
    Failed: initialJs > config.InitialJsKb * KB || initialCss > config.InitialCssKb * KB || lazy.some((chunk) => chunk.Over),
  };
};

export const parseOverrides = (argv) => {
  const read = (flag) => {
    const arg = argv.find((value) => value.startsWith(`--${flag}=`));
    if (arg === undefined) return undefined;
    const value = Number(arg.slice(flag.length + 3));
    if (!Number.isFinite(value) || value <= 0) throw new Error(`--${flag} needs a positive number of KB`);
    return value;
  };
  return { InitialJsKb: read('initial-js-kb'), InitialCssKb: read('initial-css-kb'), LazyJsKb: read('lazy-kb') };
};

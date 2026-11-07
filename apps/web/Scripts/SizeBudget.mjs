export const KB = 1024;

export const chunkName = (ref) => {
  const file = ref.slice(ref.lastIndexOf('/') + 1).replace(/\.js$/, '');
  return file.replace(/-[\w-]{8}$/, '');
};

export const lazyBudgetKb = (name, config) => {
  const match = config.LazyOverrides.find((rule) => new RegExp(rule.Pattern).test(name));
  return match === undefined ? config.LazyJsKb : match.Kb;
};

export const withOverrides = (config, overrides) => ({
  ...config,
  InitialJsKb: overrides.InitialJsKb ?? config.InitialJsKb,
  InitialCssKb: overrides.InitialCssKb ?? config.InitialCssKb,
  LazyJsKb: overrides.LazyJsKb ?? config.LazyJsKb,
  LazyOverrides:
    overrides.LazyJsKb === undefined ? config.LazyOverrides : config.LazyOverrides.map((rule) => ({ ...rule, Kb: Math.min(rule.Kb, overrides.LazyJsKb) })),
});

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

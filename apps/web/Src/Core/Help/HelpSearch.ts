import type { IHelpContent, IHelpEntry, TBlock, TInline } from '@/Core/Help/HelpTypes';

export type THelpKind = 'faq' | 'troubleshooting';

export interface IIndexedEntry {
  Kind: THelpKind;
  Group: string | null;
  Entry: IHelpEntry;
  Title: string;
  Body: string;
}

export const normalizeText = (text: string): string =>
  text
    .normalize('NFKD')
    .replace(/[̀-ًͯ-ٰٟـ]/g, '')
    .replace(/[أإآٱ]/g, 'ا')
    .replace(/ى/g, 'ي')
    .replace(/ة/g, 'ه')
    .toLowerCase();

const inlineText = (nodes: readonly TInline[]): string => nodes.map((node) => node.Text).join('');

const blockText = (block: TBlock): string => {
  if (block.Type === 'paragraph') return inlineText(block.Inline);
  if (block.Type === 'list') return block.Items.map(inlineText).join(' ');
  return block.Text;
};

export const plainText = (blocks: readonly TBlock[]): string => blocks.map(blockText).join(' ');

export const buildIndex = (content: IHelpContent): IIndexedEntry[] => [
  ...content.Faq.flatMap((group) =>
    group.Entries.map((entry): IIndexedEntry => ({
      Kind: 'faq',
      Group: group.Title,
      Entry: entry,
      Title: normalizeText(entry.Title),
      Body: normalizeText(plainText(entry.Body)),
    })),
  ),
  ...content.Troubleshooting.map((entry): IIndexedEntry => ({
    Kind: 'troubleshooting',
    Group: null,
    Entry: entry,
    Title: normalizeText(entry.Title),
    Body: normalizeText(plainText(entry.Body)),
  })),
];

export const queryTerms = (query: string): string[] =>
  normalizeText(query)
    .split(/[\s\p{P}\p{S}]+/u)
    .filter((term) => term !== '');

const TITLE_WEIGHT = 4;

const scoreOf = (item: IIndexedEntry, terms: readonly string[]): number => {
  let score = 0;
  for (const term of terms) {
    const inTitle = item.Title.includes(term);
    if (!inTitle && !item.Body.includes(term)) return 0;
    score += inTitle ? TITLE_WEIGHT : 1;
  }
  return score;
};

export const searchEntries = (index: readonly IIndexedEntry[], query: string): IIndexedEntry[] => {
  const terms = queryTerms(query);
  if (terms.length === 0) return [...index];
  return index
    .map((item, position) => ({ item, position, score: scoreOf(item, terms) }))
    .filter((hit) => hit.score > 0)
    .sort((a, b) => b.score - a.score || a.position - b.position)
    .map((hit) => hit.item);
};

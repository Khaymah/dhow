import { Fragment } from 'react';

import { docsUrl } from '@/Core/Help/DocsUrl';
import type { TBlock, TInline } from '@/Core/Help/HelpTypes';
import type { TLocale } from '@/I18n/Locale';
import { useI18n } from '@/I18n/useI18n';

const EXTERNAL_PREFIX = 'https://github.com/Khaymah/dhow';

const hrefOf = (node: Extract<TInline, { Type: 'link' }>, locale: TLocale): string | null => {
  if (node.Kind === 'doc') return docsUrl(node.Doc, locale);
  return node.Href === EXTERNAL_PREFIX || node.Href.startsWith(`${EXTERNAL_PREFIX}/`) ? node.Href : null;
};

const InlineNode = ({ node }: { node: TInline }) => {
  const { Locale, t } = useI18n();
  if (node.Type === 'code') {
    return (
      <code dir="ltr" className="rounded-sm bg-surface-inset px-1.5 py-0.5 text-code break-words">
        {node.Text}
      </code>
    );
  }
  if (node.Type === 'strong') return <strong>{node.Text}</strong>;
  if (node.Type !== 'link') return <>{node.Text}</>;
  const href = hrefOf(node, Locale);
  if (href === null) return <>{node.Text}</>;
  return (
    <a href={href} target="_blank" rel="noreferrer noopener" className="text-primary underline underline-offset-4">
      {node.Text}
      <span aria-hidden="true">{' ↗'}</span>
      <span className="sr-only">{` (${t('api.error.newTab')})`}</span>
    </a>
  );
};

export const HelpInline = ({ nodes }: { nodes: readonly TInline[] }) => (
  <>
    {nodes.map((node, index) => (
      <InlineNode key={index} node={node} />
    ))}
  </>
);

const BlockNode = ({ block }: { block: TBlock }) => {
  if (block.Type === 'paragraph') {
    return (
      <p className="m-0 text-body text-secondary">
        <HelpInline nodes={block.Inline} />
      </p>
    );
  }
  if (block.Type === 'heading') return <h5 className="m-0 text-label text-primary">{block.Text}</h5>;
  if (block.Type === 'code') {
    return (
      <pre dir="ltr" className="m-0 rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code break-words whitespace-pre-wrap">
        <code>{block.Text}</code>
      </pre>
    );
  }
  const List = block.Ordered ? 'ol' : 'ul';
  return (
    <List className={`m-0 grid gap-1.5 ps-6 text-body text-secondary ${block.Ordered ? 'list-decimal' : 'list-disc'}`}>
      {block.Items.map((item, index) => (
        <li key={index}>
          <HelpInline nodes={item} />
        </li>
      ))}
    </List>
  );
};

export const HelpBlocks = ({ blocks }: { blocks: readonly TBlock[] }) => (
  <div className="grid gap-3">
    {blocks.map((block, index) => (
      <Fragment key={index}>
        <BlockNode block={block} />
      </Fragment>
    ))}
  </div>
);

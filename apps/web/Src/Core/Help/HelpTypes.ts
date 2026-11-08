export interface IInlineText {
  Type: 'text' | 'code' | 'strong';
  Text: string;
}

export interface IInlineDocLink {
  Type: 'link';
  Kind: 'doc';
  Text: string;
  Doc: string;
}

export interface IInlineExternalLink {
  Type: 'link';
  Kind: 'external';
  Text: string;
  Href: string;
}

export type TInline = IInlineText | IInlineDocLink | IInlineExternalLink;

export interface IParagraphBlock {
  Type: 'paragraph';
  Inline: TInline[];
}

export interface IListBlock {
  Type: 'list';
  Ordered: boolean;
  Items: TInline[][];
}

export interface IHeadingBlock {
  Type: 'heading';
  Text: string;
}

export interface ICodeBlock {
  Type: 'code';
  Text: string;
}

export type TBlock = IParagraphBlock | IListBlock | IHeadingBlock | ICodeBlock;

export interface IHelpEntry {
  Id: string;
  Title: string;
  Body: TBlock[];
}

export interface IHelpGroup {
  Id: string;
  Title: string;
  Entries: IHelpEntry[];
}

export type TReleaseKind = 'highlights' | 'security' | 'actions' | 'deprecations';

export interface IReleaseSection {
  Kind: TReleaseKind;
  Title: string;
  Body: TBlock[];
}

export interface IRelease {
  Version: string;
  Title: string;
  Sections: IReleaseSection[];
}

export interface IHelpContent {
  Faq: IHelpGroup[];
  Troubleshooting: IHelpEntry[];
  Releases: IRelease[];
}

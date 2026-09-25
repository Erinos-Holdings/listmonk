import React, { createContext, useContext } from 'react';
import { z } from 'zod';

import { Avatar, AvatarPropsSchema } from '@usewaypoint/block-avatar';
import { ColumnsContainer as BaseColumnsContainer } from '@usewaypoint/block-columns-container';
import { Container as BaseContainer } from '@usewaypoint/block-container';
import { Divider, DividerPropsSchema } from '@usewaypoint/block-divider';
import { Heading, HeadingPropsSchema } from '@usewaypoint/block-heading';
import { HtmlPropsSchema } from '@usewaypoint/block-html';
import { Image } from '@usewaypoint/block-image';
import { Spacer, SpacerPropsSchema } from '@usewaypoint/block-spacer';
import { Text, TextPropsSchema } from '@usewaypoint/block-text';
import {
  buildBlockComponent,
  buildBlockConfigurationDictionary,
  buildBlockConfigurationSchema,
} from '@usewaypoint/document-core';

import Button from '../blocks/Button/Button';
import ButtonPropsSchema from '../blocks/Button/ButtonPropsSchema';
import ColumnsContainerPropsSchema from '../blocks/ColumnsContainer/ColumnsContainerPropsSchema';
import ContainerPropsSchema from '../blocks/Container/ContainerPropsSchema';
import EmailLayoutPropsSchema, { EmailLayoutProps, getBackdropPadding } from '../blocks/EmailLayout/EmailLayoutPropsSchema';
import { FONT_FAMILIES } from '../blocks/helpers/fontFamily';
import { ImgPropsSchema } from '../blocks/Img/ImgPropsSchema';
import { OfficialRenderContext, parseReference } from '../blocks/OfficialFooter/OfficialContext';
import OfficialFooterPropsSchema from '../blocks/OfficialFooter/OfficialFooterPropsSchema';
import { CANVAS_WIDTH } from '../canvasWidth';
import { officialProjection } from '../../official/projection';
import { officialLang, officialSlug, resolveOfficial } from '../../official/resolve';

/**
 * A local fork of @usewaypoint/email-builder's Reader (MIT, (c) 2024 Waypoint
 * (Metaccountant, Inc.)).
 *
 * The packaged Reader builds its block dictionary privately and never exports
 * it, so there is no way to point the Button entry at our own component — the
 * only reason this file exists. Everything except Button is upstream's
 * component, and the three container readers below are transcriptions of
 * upstream's, which the package does not export either.
 *
 * Keep this in step with the editor dictionary in ../editor/core.tsx: a block
 * that renders one way on the canvas and another in the exported HTML is the
 * failure mode this file makes possible.
 */

const ReaderContext = createContext<TReaderDocument>({});

function useReaderDocument() {
  return useContext(ReaderContext);
}

function ColumnsContainerReader({ style, props }: z.infer<typeof ColumnsContainerPropsSchema>) {
  const { columns, ...restProps } = props ?? {};
  let cols = undefined;
  if (columns) {
    cols = columns.map((col) => col.childrenIds.map((childId) => <ReaderBlock key={childId} id={childId} />));
  }
  return <BaseColumnsContainer props={restProps} columns={cols} style={style} />;
}

function ContainerReader({ style, props }: z.infer<typeof ContainerPropsSchema>) {
  const childrenIds = props?.childrenIds ?? [];
  return (
    <BaseContainer style={style}>
      {childrenIds.map((childId) => (
        <ReaderBlock key={childId} id={childId} />
      ))}
    </BaseContainer>
  );
}

function getFontFamily(fontFamily: EmailLayoutProps['fontFamily']) {
  const f = fontFamily ?? 'MODERN_SANS';
  return FONT_FAMILIES.find((font) => font.key === f)?.value;
}

// Transcription of upstream's Html component with one addition: the marker
// attribute. It fences the block's user-authored contents off from the compile
// post-processor (postProcess.ts transformSimpleDivBlocks), which must not rewrite
// user divs into table cells — td has no margin, no inline-block, no floats.
// The marker rides the padding wrapper itself so the WRAPPER stays convertible
// (its padding is builder-owned); only strict descendants are fenced.
function HtmlReader({ style, props }: z.infer<typeof HtmlPropsSchema>) {
  const contents = props?.contents;
  const padding = style?.padding;
  const cssStyle: React.CSSProperties = {
    color: style?.color ?? undefined,
    backgroundColor: style?.backgroundColor ?? undefined,
    fontFamily: getFontFamily(style?.fontFamily),
    fontSize: style?.fontSize ?? undefined,
    textAlign: style?.textAlign ?? undefined,
    padding: padding ? `${padding.top}px ${padding.right}px ${padding.bottom}px ${padding.left}px` : undefined,
  };
  if (!contents) {
    return <div data-lm-user-html="true" style={cssStyle} />;
  }
  return <div data-lm-user-html="true" style={cssStyle} dangerouslySetInnerHTML={{ __html: contents }} />;
}

function getBorder({ borderColor }: EmailLayoutProps) {
  if (!borderColor) {
    return undefined;
  }
  return `1px solid ${borderColor}`;
}

function EmailLayoutReader(props: EmailLayoutProps) {
  const childrenIds = props.childrenIds ?? [];
  return (
    <>
      {props.linkColor && <style>{`a { color: ${props.linkColor}; }`}</style>}
      <div
        style={{
          backgroundColor: props.backdropColor ?? '#F5F5F5',
          color: props.textColor ?? '#262626',
          fontFamily: getFontFamily(props.fontFamily),
          fontSize: '16px',
          fontWeight: '400',
          letterSpacing: '0.15008px',
          lineHeight: '1.5',
          margin: '0',
          padding: `${getBackdropPadding(props.backdropPadding)}px 0`,
          minHeight: '100%',
          width: '100%',
        }}
      >
        <table
          align="center"
          width="100%"
          style={{
            margin: '0 auto',
            maxWidth: `${CANVAS_WIDTH}px`,
            backgroundColor: props.canvasColor ?? '#FFFFFF',
            borderRadius: props.borderRadius ?? undefined,
            border: getBorder(props),
          }}
          role="presentation"
          cellSpacing="0"
          cellPadding="0"
          border={0}
        >
          <tbody>
            <tr style={{ width: '100%' }}>
              <td>
                {childrenIds.map((childId) => (
                  <ReaderBlock key={childId} id={childId} />
                ))}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </>
  );
}

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D4. The compile of an OfficialFooter block is
// its marker, the resolved reference's ROOT CHILDREN rendered in place (the reference's own
// EmailLayout props are ignored, so the campaign's backdrop/font govern), and the end marker.
// React cannot render a comment node, so the markers are two empty placeholder elements that
// renderHtmlWithMeta (utils.tsx) replaces with the comment pair BEFORE postProcess runs -- no
// wrapper element survives, which is what makes the compile byte-identical to the same
// document with the reference's blocks pasted in (I2).
//   ok / duplicate  <!-- official:<kind>:<lang>:<brand>:<hash> --> ...children... <!-- /official -->
//   missing         <!-- official:<kind>:<lang>:<brand>:missing --><!-- /official -->
//   no-context      <!-- official:<kind>:<lang>:<brand>:no-context --><!-- /official -->
//   none (curated)  nothing at all, marker included
// `brand` is `-` for the corporate kind. An OfficialFooter met INSIDE a reference renders
// nothing (depth 1 only).
export const OFFICIAL_OPEN_TAG = 'lm-official-open';
export const OFFICIAL_CLOSE_TAG = 'lm-official-close';

function officialMarker(marker: string, inner: React.ReactNode) {
  return (
    <>
      {React.createElement(OFFICIAL_OPEN_TAG, { 'data-marker': marker })}
      {inner}
      {React.createElement(OFFICIAL_CLOSE_TAG)}
    </>
  );
}

// A reference document's root children, rendered by the reader against THAT document (a nested
// ReaderContext), with the nested flag set so an OfficialFooter inside it renders nothing. The
// canvas uses this too (OfficialFooterEditor), so the canvas and the compile render one way.
export function OfficialReferenceChildren({ document }: { document: TReaderDocument }) {
  const official = useContext(OfficialRenderContext);
  const root = (document as Record<string, any>).root;
  const childrenIds: string[] = (root && root.data && root.data.childrenIds) || [];
  return (
    <OfficialRenderContext.Provider value={{ refs: official?.refs ?? [], context: official?.context ?? null, nested: true }}>
      <ReaderContext.Provider value={document}>
        {childrenIds.map((childId) => (
          <ReaderBlock key={childId} id={childId} />
        ))}
      </ReaderContext.Provider>
    </OfficialRenderContext.Provider>
  );
}

function OfficialFooterReader({ props }: z.infer<typeof OfficialFooterPropsSchema>) {
  const official = useContext(OfficialRenderContext);
  const kind = props?.kind;
  if (official?.nested) {
    // eslint-disable-next-line no-console
    console.warn('OfficialFooter inside an official reference renders nothing (depth 1 only)');
    return <></>;
  }
  if (!official || (kind !== 'corporate' && kind !== 'brand')) {
    return <></>;
  }

  const res = resolveOfficial(kind, official.context, official.refs);
  if (res.status === 'none') {
    return <></>;
  }
  const lang = officialLang(official.context?.lang).toLowerCase();
  const brand = kind === 'corporate' ? '-' : officialSlug(official.context?.brand) || '-';
  const prefix = `official:${kind}:${lang}:${brand}`;

  const doc = res.ref ? parseReference(res.ref.body_source) : null;
  if ((res.status === 'ok' || res.status === 'duplicate') && doc) {
    return officialMarker(`${prefix}:${officialProjection(doc).hash}`, <OfficialReferenceChildren document={doc as TReaderDocument} />);
  }
  // missing / no-context (or a reference whose body_source does not parse): an EMPTY pair.
  return officialMarker(`${prefix}:${res.status === 'no-context' ? 'no-context' : 'missing'}`, null);
}

const READER_DICTIONARY = buildBlockConfigurationDictionary({
  ColumnsContainer: {
    schema: ColumnsContainerPropsSchema,
    Component: ColumnsContainerReader,
  },
  Container: {
    schema: ContainerPropsSchema,
    Component: ContainerReader,
  },
  EmailLayout: {
    schema: EmailLayoutPropsSchema,
    Component: EmailLayoutReader,
  },
  //
  Avatar: {
    schema: AvatarPropsSchema,
    Component: Avatar,
  },
  Button: {
    schema: ButtonPropsSchema,
    Component: Button,
  },
  Divider: {
    schema: DividerPropsSchema,
    Component: Divider,
  },
  Heading: {
    schema: HeadingPropsSchema,
    Component: Heading,
  },
  Html: {
    schema: HtmlPropsSchema,
    Component: HtmlReader,
  },
  Image: {
    schema: ImgPropsSchema,
    Component: Image,
  },
  Spacer: {
    schema: SpacerPropsSchema,
    Component: Spacer,
  },
  Text: {
    schema: TextPropsSchema,
    Component: Text,
  },
  OfficialFooter: {
    schema: OfficialFooterPropsSchema,
    Component: OfficialFooterReader,
  },
});

// Exported for parity with upstream's Reader. NOTE: nothing validates against
// these — buildBlockComponent spreads `data` into the component unparsed, which
// is why an unknown prop reaches a block instead of being stripped. That is what
// let the upstream Button silently ignore customWidth, and why rendering it took
// a local component rather than a schema change.
export const ReaderBlockSchema = buildBlockConfigurationSchema(READER_DICTIONARY);
export type TReaderBlock = z.infer<typeof ReaderBlockSchema>;

export const ReaderDocumentSchema = z.record(z.string(), ReaderBlockSchema);
export type TReaderDocument = Record<string, TReaderBlock>;

const BaseReaderBlock = buildBlockComponent(READER_DICTIONARY);

export function ReaderBlock({ id }: { id: string }) {
  const document = useReaderDocument();
  return <BaseReaderBlock {...document[id]} />;
}

export function Reader({ document, rootBlockId }: { document: TReaderDocument; rootBlockId: string }) {
  return (
    <ReaderContext.Provider value={document}>
      <ReaderBlock id={rootBlockId} />
    </ReaderContext.Provider>
  );
}

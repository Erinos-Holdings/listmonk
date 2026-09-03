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
import EmailLayoutPropsSchema, { EmailLayoutProps } from '../blocks/EmailLayout/EmailLayoutPropsSchema';
import { FONT_FAMILIES } from '../blocks/helpers/fontFamily';
import { ImgPropsSchema } from '../blocks/Img/ImgPropsSchema';
import { CANVAS_WIDTH } from '../canvasWidth';

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
// attribute. It fences the block's user-authored contents off from the Outlook
// post-processor (outlook.ts transformSimpleDivBlocks), which must not rewrite
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
        padding: '32px 0',
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
  );
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

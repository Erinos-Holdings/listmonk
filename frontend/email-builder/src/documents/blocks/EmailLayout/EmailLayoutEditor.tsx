import React from 'react';

import { useCurrentBlockId } from '../../editor/EditorBlock';
import { setDocument, setSelectedBlockId, useDocument } from '../../editor/EditorContext';
import EditorChildrenIds from '../helpers/EditorChildrenIds';

import { EmailLayoutProps } from './EmailLayoutPropsSchema';
import { CANVAS_WIDTH } from '../../canvasWidth';
import { TEXT_FLOW_TAG_NAMES } from '../../../postProcess';

// PARAGRAPH-SPACING-SPEC D3: the canvas mirror of the compile's normalizeTextMargins
// (postProcess.ts). The marker and `--lm-text-size` sit on EditorBlockWrapper's Box, which
// wraps the Text block's own padded div — so the rule reaches through it to that div's
// direct text-flow children (`> :last-child`: the Box's first child is the TuneMenu while a
// block is selected; the block is always last). Same tag set as the compile, same px size
// (the BLOCK's, not each child's em — a markdown <h1> is UA 2em). Longhand `!important`
// is what beats the vendored blockquote's inline `margin: 0 0 12px 0`; side margins are
// never touched. The `:has` rule covers a Text block ending in a non-text child (<hr>):
// the compile zeroes the last TEXT-FLOW child, not the last child. It is a separate rule so
// a browser without `:has` drops only it. Known divergence, accepted: a Text block whose
// markdown carries a raw <div> is not a compile candidate at all, but the canvas still
// applies the rule (excluding it would put `:has` in the main rules).
const TEXT_FLOW = `:is(${TEXT_FLOW_TAG_NAMES.map((t) => t.toLowerCase()).join(',')})`;
const TEXT_MARGIN_CSS = [
  `.lm-email-canvas [data-lm-text] > :last-child > ${TEXT_FLOW} { margin-top: 0 !important; margin-bottom: var(--lm-text-size) !important; }`,
  `.lm-email-canvas [data-lm-text] > :last-child > ${TEXT_FLOW}:last-child { margin-bottom: 0 !important; }`,
  `.lm-email-canvas [data-lm-text] > :last-child > ${TEXT_FLOW}:not(:has(~ ${TEXT_FLOW})) { margin-bottom: 0 !important; }`,
].join('\n');

function getFontFamily(fontFamily: EmailLayoutProps['fontFamily']) {
  const f = fontFamily ?? 'MODERN_SANS';
  switch (f) {
    case 'MODERN_SANS':
      return '"Helvetica Neue", "Arial Nova", "Nimbus Sans", Arial, sans-serif';
    case 'BOOK_SANS':
      return 'Optima, Candara, "Noto Sans", source-sans-pro, sans-serif';
    case 'ORGANIC_SANS':
      return 'Seravek, "Gill Sans Nova", Ubuntu, Calibri, "DejaVu Sans", source-sans-pro, sans-serif';
    case 'GEOMETRIC_SANS':
      return 'Avenir, "Avenir Next LT Pro", Montserrat, Corbel, "URW Gothic", source-sans-pro, sans-serif';
    case 'HEAVY_SANS':
      return 'Bahnschrift, "DIN Alternate", "Franklin Gothic Medium", "Nimbus Sans Narrow", sans-serif-condensed, sans-serif';
    case 'ROUNDED_SANS':
      return 'ui-rounded, "Hiragino Maru Gothic ProN", Quicksand, Comfortaa, Manjari, "Arial Rounded MT Bold", Calibri, source-sans-pro, sans-serif';
    case 'MODERN_SERIF':
      return 'Charter, "Bitstream Charter", "Sitka Text", Cambria, serif';
    case 'BOOK_SERIF':
      return '"Iowan Old Style", "Palatino Linotype", "URW Palladio L", P052, serif';
    case 'MONOSPACE':
      return '"Nimbus Mono PS", "Courier New", "Cutive Mono", monospace';
  }
}

export default function EmailLayoutEditor(props: EmailLayoutProps) {
  const childrenIds = props.childrenIds ?? [];
  const document = useDocument();
  const currentBlockId = useCurrentBlockId();

  return (
    <>
      {props.linkColor && <style>{`.lm-email-canvas a { color: ${props.linkColor}; }`}</style>}
      <style>{TEXT_MARGIN_CSS}</style>
      <div
        className="lm-email-canvas"
        onClick={() => {
          setSelectedBlockId(null);
        }}
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
          width: '100%',
          minHeight: '100%',
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
            border: (() => {
              const v = props.borderColor;
              if (!v) {
                return undefined;
              }
              return `1px solid ${v}`;
            })(),
          }}
          role="presentation"
          cellSpacing="0"
          cellPadding="0"
          border={0}
        >
          <tbody>
            <tr style={{ width: '100%' }}>
              <td>
                <EditorChildrenIds
                  childrenIds={childrenIds}
                  onChange={({ block, blockId, childrenIds }) => {
                    setDocument({
                      [blockId]: block,
                      [currentBlockId]: {
                        type: 'EmailLayout',
                        data: {
                          ...document[currentBlockId].data,
                          childrenIds: childrenIds,
                        },
                      },
                    });
                    setSelectedBlockId(blockId);
                  }}
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </>
  );
}

import React, { CSSProperties } from 'react';

import { ButtonPropsDefaults } from '@usewaypoint/block-button';

import { FONT_FAMILIES } from '../helpers/fontFamily';
import { BUTTON_BORDER_COLOR_DEFAULT, ButtonProps } from './ButtonPropsSchema';

/**
 * A local fork of @usewaypoint/block-button's renderer (MIT, (c) 2024 Waypoint
 * (Metaccountant, Inc.)).
 *
 * It exists because the upstream component has no width or height of its own and
 * is reachable neither from the editor dictionary nor from the packaged Reader,
 * so a custom prop could be stored but never rendered.
 *
 * With customWidth, customHeight and borderSize unset this emits byte-identical
 * markup to upstream, which is what keeps already-saved Button blocks rendering
 * unchanged. Keep it that way when rebasing.
 */

function getFontFamily(fontFamily: NonNullable<ButtonProps['style']>['fontFamily']) {
  return FONT_FAMILIES.find((f) => f.key === fontFamily)?.value;
}

function getPadding(padding: NonNullable<ButtonProps['style']>['padding']) {
  return padding ? `${padding.top}px ${padding.right}px ${padding.bottom}px ${padding.left}px` : undefined;
}

function getRoundedCorners(props: ButtonProps['props']) {
  const buttonStyle = props?.buttonStyle ?? ButtonPropsDefaults.buttonStyle;
  switch (buttonStyle) {
    case 'rectangle':
      return undefined;
    case 'pill':
      return 64;
    case 'rounded':
    default:
      return 4;
  }
}

function getButtonSizePadding(props: ButtonProps['props']): [number, number] {
  const size = props?.size ?? ButtonPropsDefaults.size;
  switch (size) {
    case 'x-small':
      return [4, 8];
    case 'small':
      return [8, 12];
    case 'large':
      return [16, 32];
    case 'medium':
    default:
      return [12, 20];
  }
}

export default function Button({ style, props }: ButtonProps) {
  const text = props?.text ?? ButtonPropsDefaults.text;
  const url = props?.url ?? ButtonPropsDefaults.url;
  const fullWidth = props?.fullWidth ?? ButtonPropsDefaults.fullWidth;
  const buttonTextColor = props?.buttonTextColor ?? ButtonPropsDefaults.buttonTextColor;
  const buttonBackgroundColor = props?.buttonBackgroundColor ?? ButtonPropsDefaults.buttonBackgroundColor;

  // A fixed width is meaningless while the Full/Auto toggle is on Full, which
  // already stretches the button to its container.
  const customWidth = fullWidth ? 0 : props?.customWidth ?? 0;
  const customHeight = props?.customHeight ?? 0;
  const borderSize = props?.borderSize ?? 0;
  const borderColor = props?.borderColor ?? BUTTON_BORDER_COLOR_DEFAULT;

  // Size drives the padding until a custom dimension takes over that axis: with
  // an explicit box the text is centred inside it instead of being spaced out by
  // padding, which is what keeps the label at its own font size either way.
  const [sizePaddingV, sizePaddingH] = getButtonSizePadding(props);
  const paddingV = customHeight > 0 ? 0 : sizePaddingV;
  const paddingH = customWidth > 0 ? 0 : sizePaddingH;

  // Word honours neither `width` nor horizontal padding on an <a>, so the
  // letter-spacing on the hidden <i>s below is the only thing giving Outlook a
  // padded-looking button. Keep feeding it the Size-derived padding even once a
  // custom width has zeroed the CSS padding, or the button collapses to bare
  // text on a colour swatch there. Outlook still cannot honour the exact px box
  // — that needs the EmailLayout "Outlook compatibility" toggle, which rebuilds
  // the button as VML and is off by default.
  const msoPaddingH = sizePaddingH;
  const textRaise = (msoPaddingH * 2 * 3) / 4;

  const wrapperStyle: CSSProperties = {
    backgroundColor: style?.backgroundColor ?? undefined,
    textAlign: style?.textAlign ?? undefined,
    padding: getPadding(style?.padding),
  };

  const linkStyle: CSSProperties = {
    color: buttonTextColor,
    fontSize: style?.fontSize ?? 16,
    fontFamily: getFontFamily(style?.fontFamily),
    fontWeight: style?.fontWeight ?? 'bold',
    backgroundColor: buttonBackgroundColor,
    borderRadius: getRoundedCorners(props),
    display: fullWidth ? 'block' : 'inline-block',
    padding: `${paddingV}px ${paddingH}px`,
    textDecoration: 'none',
  };

  if (customWidth > 0) {
    linkStyle.width = `${customWidth}px`;
    linkStyle.boxSizing = 'border-box';
    linkStyle.textAlign = 'center';
  }

  if (customHeight > 0) {
    // line-height matching the box height is what vertically centres a
    // single-line label in Outlook and Gmail alike; flexbox is not an option.
    linkStyle.height = `${customHeight}px`;
    linkStyle.lineHeight = `${customHeight}px`;
    linkStyle.boxSizing = 'border-box';
  }

  if (borderSize > 0) {
    // The border sits on the same <a> as the background and border-radius, so
    // it follows the Style corner treatment (rectangle/rounded/pill) on its
    // own.
    linkStyle.border = `${borderSize}px solid ${borderColor}`;
    if (customHeight > 0) {
      // With a custom height the box is border-box, so the content area loses
      // the border on both edges — shrink the centring line box to match or
      // the label sits below centre.
      linkStyle.lineHeight = `${Math.max(0, customHeight - 2 * borderSize)}px`;
    }
  }

  if (customWidth > 0 || customHeight > 0) {
    // A fixed box only holds one line: two line boxes at line-height:<height>
    // overflow a height:<height> button, and the background sits on the <a>, so
    // the text escapes the coloured area. Keeping it on one line turns that into
    // a horizontal overrun, which is visible on the canvas rather than only in
    // whichever client resolves a wider font from the stack.
    linkStyle.whiteSpace = 'nowrap';
  }

  return (
    <div style={wrapperStyle}>
      <a href={url} style={linkStyle} target="_blank">
        <span
          dangerouslySetInnerHTML={{
            __html: `<!--[if mso]><i style="letter-spacing: ${msoPaddingH}px;mso-font-width:-100%;mso-text-raise:${textRaise}" hidden>&nbsp;</i><![endif]-->`,
          }}
        />
        <span>{text}</span>
        <span
          dangerouslySetInnerHTML={{
            __html: `<!--[if mso]><i style="letter-spacing: ${msoPaddingH}px;mso-font-width:-100%" hidden>&nbsp;</i><![endif]-->`,
          }}
        />
      </a>
    </div>
  );
}

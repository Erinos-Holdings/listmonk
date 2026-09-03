import {
  VerticalAlignBottomOutlined,
  VerticalAlignCenterOutlined,
  VerticalAlignTopOutlined,
} from '@mui/icons-material';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import WarningAmberOutlined from '@mui/icons-material/WarningAmberOutlined';
import { Checkbox, FormControlLabel, Stack, ToggleButton } from '@mui/material';
import { ImageProps } from '@usewaypoint/block-image';
import React, { useEffect, useRef, useState } from 'react';

import { isAltMissing } from '../../../../documents/blocks/Img/altText';
import { autoFillCap, createImageWidthMeasurer, decideWidth } from '../../../../documents/blocks/Img/imageWidth';
import { ImgPropsSchema, ListmonkImageProps } from '../../../../documents/editor/core';
import BaseSidebarPanel from './helpers/BaseSidebarPanel';
import RadioGroupInput from './helpers/inputs/RadioGroupInput';
import TextDimensionInput from './helpers/inputs/TextDimensionInput';
import TextInput from './helpers/inputs/TextInput';
import MultiStylePropertyPanel from './helpers/style-inputs/MultiStylePropertyPanel';

type ImageSidebarPanelProps = {
  data: ImageProps;
  setData: (v: ImageProps) => void;
};
// Fired by the host page's media picker write-back (VisualEditor.vue::onMediaSelect) on
// the Source URL input AFTER it sets the value. React's onChange only fires when the DOM
// value differs from the tracked one, so a same-URL re-select would otherwise never
// re-run the width auto-fill (IMAGE-WIDTH-SPEC §4 / F3).
export const MEDIA_SELECTED_EVENT = 'lm-media-selected';

export default function ImageSidebarPanel({ data, setData }: ImageSidebarPanelProps) {
  const [, setErrors] = useState<Zod.ZodError | null>(null);

  // Width auto-fill state (IMAGE-WIDTH-SPEC Part A). `dataRef` is the latest block data,
  // read at measure-resolve time so the write never spreads a stale closure over alt/height
  // typed while a slow image loaded (F4). `autoPrevRef` is what the last auto-fill wrote in
  // THIS panel session — transient by design: a re-opened block with a stored width is
  // authored. `widthFillSeq` remounts the uncontrolled Width input so it shows what the
  // document holds after an auto-fill (keying on the width itself would remount on every
  // keystroke and drop focus).
  const dataRef = useRef(data);
  dataRef.current = data;
  const autoPrevRef = useRef<number | null>(null);
  const measureRef = useRef(createImageWidthMeasurer());
  const [widthFillSeq, setWidthFillSeq] = useState(0);
  const urlFieldRef = useRef<HTMLDivElement>(null);
  // A typed URL measures after a short pause (every keystroke would otherwise request a
  // partial URL from whatever host it resolves to); the media picker path is immediate.
  const typedMeasureTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelTypedMeasure = () => {
    if (typedMeasureTimer.current !== null) {
      clearTimeout(typedMeasureTimer.current);
      typedMeasureTimer.current = null;
    }
  };
  useEffect(() => cancelTypedMeasure, []);

  const updateData = (d: unknown) => {
    const res = ImgPropsSchema.safeParse(d);
    if (res.success) {
      setData(res.data as ImageProps);
      setErrors(null);
    } else {
      setErrors(res.error);
    }
  };

  const props = (data && (data as ListmonkImageProps).props) || {};

  // D3.3: on a URL set, fill Width from the image's natural width when Width is empty or
  // still holds the previous auto-fill; a typed width survives. Failure (404, non-image,
  // SVG with no intrinsic size, a URL still being typed) resolves null and writes nothing
  // (D3.5). A later URL set supersedes an in-flight measure.
  const fillWidthFromImage = (url: string | null) => {
    if (!url) {
      return;
    }
    measureRef.current(url).then((measured) => {
      const latest = dataRef.current as ListmonkImageProps;
      const cap = autoFillCap(latest?.style?.padding);
      const width = decideWidth(latest?.props?.width, autoPrevRef.current, measured, cap);
      if (width === null) {
        return;
      }
      autoPrevRef.current = width;
      updateData({ ...latest, props: { ...latest?.props, width } });
      setWidthFillSeq((n) => n + 1);
    });
  };

  useEffect(() => {
    const el = urlFieldRef.current;
    if (!el) {
      return undefined;
    }
    const onMediaSelected = (ev: Event) => {
      const detail = (ev as CustomEvent<string>).detail;
      const url = typeof detail === 'string' && detail.trim().length > 0
        ? detail.trim()
        : (dataRef.current as ListmonkImageProps)?.props?.url ?? null;
      cancelTypedMeasure();
      fillWidthFromImage(url);
    };
    el.addEventListener(MEDIA_SELECTED_EVENT, onMediaSelected);
    return () => el.removeEventListener(MEDIA_SELECTED_EVENT, onMediaSelected);
    // fillWidthFromImage reads only refs; the listener needs binding once per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <BaseSidebarPanel title="Image block">
      <div ref={urlFieldRef}>
        <TextInput
          label="Source URL"
          className="image-url"
          defaultValue={data.props?.url ?? ''}
          onChange={(v) => {
            const url = v.trim().length === 0 ? null : v.trim();
            updateData({ ...data, props: { ...data.props, url } });
            cancelTypedMeasure();
            typedMeasureTimer.current = setTimeout(() => {
              typedMeasureTimer.current = null;
              fillWidthFromImage(url);
            }, 300);
          }}
        />
      </div>
      <a href="#" class="select-media"
        style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', marginTop: '5px' }}
        onClick={(e) => {
        // @ts-ignore
        window.parent.postMessage('visualeditor.select-media', '*');
        e.preventDefault();
      }}><CloudUploadIcon style={{fontSize: '1rem'}} /> Select media</a>

      <TextInput
        label="Alt text"
        defaultValue={data.props?.alt ?? ''}
        onChange={(alt) => updateData({ ...data, props: { ...data.props, alt } })}
      />
      {isAltMissing(data.props) && (
        <div style={{
          display: 'flex', gap: '0.4rem', alignItems: 'flex-start',
          color: '#b45309', fontSize: '0.75rem', lineHeight: 1.4,
        }}>
          <WarningAmberOutlined style={{ fontSize: '1rem', flexShrink: 0 }} />
          <span>
            No alt text. Outlook blocks remote images by default, so until the reader
            loads images the alt text IS the email — and screen readers need it. Type a
            description, or type-and-clear to mark the image decorative (empty alt).
          </span>
        </div>
      )}
      <TextInput
        label="Click through URL"
        defaultValue={data.props?.linkHref ?? ''}
        onChange={(v) => {
          const linkHref = v.trim().length === 0 ? null : v.trim();
          updateData({ ...data, props: { ...data.props, linkHref } });
        }}
      />
      <Stack direction="row" spacing={2}>
        <TextDimensionInput
          key={`width-${widthFillSeq}`}
          label="Width"
          defaultValue={data.props?.width}
          onChange={(width) => updateData({ ...data, props: { ...data.props, width } })}
        />
        <TextDimensionInput
          label="Height"
          defaultValue={data.props?.height}
          onChange={(height) => updateData({ ...data, props: { ...data.props, height } })}
        />
      </Stack>

      <RadioGroupInput
        label="Alignment"
        defaultValue={data.props?.contentAlignment ?? 'middle'}
        onChange={(contentAlignment) => updateData({ ...data, props: { ...data.props, contentAlignment } })}
      >
        <ToggleButton value="top">
          <VerticalAlignTopOutlined fontSize="small" />
        </ToggleButton>
        <ToggleButton value="middle">
          <VerticalAlignCenterOutlined fontSize="small" />
        </ToggleButton>
        <ToggleButton value="bottom">
          <VerticalAlignBottomOutlined fontSize="small" />
        </ToggleButton>
      </RadioGroupInput>

      <FormControlLabel
        control={
          <Checkbox
            size="small"
            checked={Boolean(props.embed)}
            onChange={(e) => updateData({ ...data, props: { ...data.props, embed: e.target.checked } })}
          />
        }
        label="Embed inline (CID)"
      />

      <MultiStylePropertyPanel
        names={['backgroundColor', 'textAlign', 'padding']}
        value={data.style}
        onChange={(style) => updateData({ ...data, style })}
      />
    </BaseSidebarPanel>
  );
}

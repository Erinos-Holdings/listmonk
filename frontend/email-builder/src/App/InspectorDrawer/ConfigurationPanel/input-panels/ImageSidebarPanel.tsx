import {
  VerticalAlignBottomOutlined,
  VerticalAlignCenterOutlined,
  VerticalAlignTopOutlined,
} from '@mui/icons-material';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import WarningAmberOutlined from '@mui/icons-material/WarningAmberOutlined';
import { Checkbox, FormControlLabel, Stack, ToggleButton } from '@mui/material';
import { ImageProps } from '@usewaypoint/block-image';
import React, { useState } from 'react';

import { isAltMissing } from '../../../../documents/blocks/Img/altText';
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
export default function ImageSidebarPanel({ data, setData }: ImageSidebarPanelProps) {
  const [, setErrors] = useState<Zod.ZodError | null>(null);

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

  return (
    <BaseSidebarPanel title="Image block">
      <TextInput
        label="Source URL"
        className="image-url"
        defaultValue={data.props?.url ?? ''}
        onChange={(v) => {
          const url = v.trim().length === 0 ? null : v.trim();
          updateData({ ...data, props: { ...data.props, url } });
        }}
      />
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

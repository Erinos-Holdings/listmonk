import React, { useState } from 'react';

import { HeightOutlined, LineWeightOutlined, SwapHorizOutlined } from '@mui/icons-material';
import { ToggleButton } from '@mui/material';
import { ButtonPropsDefaults } from '@usewaypoint/block-button';

import ButtonPropsSchema, {
  BUTTON_BORDER_COLOR_DEFAULT,
  ButtonProps,
} from '../../../../documents/blocks/Button/ButtonPropsSchema';
import BaseSidebarPanel from './helpers/BaseSidebarPanel';
import ColorInput from './helpers/inputs/ColorInput';
import RadioGroupInput from './helpers/inputs/RadioGroupInput';
import SliderInput from './helpers/inputs/SliderInput';
import TextInput from './helpers/inputs/TextInput';
import MultiStylePropertyPanel from './helpers/style-inputs/MultiStylePropertyPanel';

type ButtonSidebarPanelProps = {
  data: ButtonProps;
  setData: (v: ButtonProps) => void;
};
export default function ButtonSidebarPanel({ data, setData }: ButtonSidebarPanelProps) {
  const [, setErrors] = useState<Zod.ZodError | null>(null);

  const updateData = (d: unknown) => {
    const res = ButtonPropsSchema.safeParse(d);
    if (res.success) {
      setData(res.data);
      setErrors(null);
    } else {
      setErrors(res.error);
    }
  };

  const text = data.props?.text ?? ButtonPropsDefaults.text;
  const url = data.props?.url ?? ButtonPropsDefaults.url;
  const fullWidth = data.props?.fullWidth ?? ButtonPropsDefaults.fullWidth;
  const size = data.props?.size ?? ButtonPropsDefaults.size;
  const buttonStyle = data.props?.buttonStyle ?? ButtonPropsDefaults.buttonStyle;
  const buttonTextColor = data.props?.buttonTextColor ?? ButtonPropsDefaults.buttonTextColor;
  const buttonBackgroundColor = data.props?.buttonBackgroundColor ?? ButtonPropsDefaults.buttonBackgroundColor;
  // 0 means "size the button from its text", which is what every document saved
  // before these props existed does.
  const customWidth = data.props?.customWidth ?? 0;
  const customHeight = data.props?.customHeight ?? 0;
  // 0 means "no border", which is what every document saved before these props
  // existed does.
  const borderSize = data.props?.borderSize ?? 0;
  const borderColor = data.props?.borderColor ?? BUTTON_BORDER_COLOR_DEFAULT;

  return (
    <BaseSidebarPanel title="Button block">
      <TextInput
        label="Text"
        defaultValue={text}
        onChange={(text) => updateData({ ...data, props: { ...data.props, text } })}
      />
      <TextInput
        label="Url"
        defaultValue={url}
        onChange={(url) => updateData({ ...data, props: { ...data.props, url } })}
      />
      <RadioGroupInput
        label="Width"
        defaultValue={fullWidth ? 'FULL_WIDTH' : 'AUTO'}
        onChange={(v) => updateData({ ...data, props: { ...data.props, fullWidth: v === 'FULL_WIDTH' } })}
      >
        <ToggleButton value="FULL_WIDTH">Full</ToggleButton>
        <ToggleButton value="AUTO">Auto</ToggleButton>
      </RadioGroupInput>
      {/* Full stretches the button to its container, so a fixed width is inert
          there and the renderer ignores it. Hide the control rather than leave a
          slider that moves and changes nothing. The value stays in the document,
          so switching back to Auto restores it. */}
      {!fullWidth && (
        <SliderInput
          label="Custom button width"
          iconLabel={<SwapHorizOutlined sx={{ fontSize: 16 }} />}
          units="px"
          step={10}
          min={0}
          max={600}
          zeroLabel="Auto"
          defaultValue={customWidth}
          onChange={(customWidth) => updateData({ ...data, props: { ...data.props, customWidth } })}
        />
      )}
      <SliderInput
        label="Custom button height"
        iconLabel={<HeightOutlined sx={{ fontSize: 16 }} />}
        units="px"
        step={4}
        min={0}
        max={120}
        zeroLabel="Auto"
        defaultValue={customHeight}
        onChange={(customHeight) => updateData({ ...data, props: { ...data.props, customHeight } })}
      />
      <RadioGroupInput
        label="Size"
        defaultValue={size}
        onChange={(size) => updateData({ ...data, props: { ...data.props, size } })}
      >
        <ToggleButton value="x-small">Xs</ToggleButton>
        <ToggleButton value="small">Sm</ToggleButton>
        <ToggleButton value="medium">Md</ToggleButton>
        <ToggleButton value="large">Lg</ToggleButton>
      </RadioGroupInput>
      <RadioGroupInput
        label="Style"
        defaultValue={buttonStyle}
        onChange={(buttonStyle) => updateData({ ...data, props: { ...data.props, buttonStyle } })}
      >
        <ToggleButton value="rectangle">Rectangle</ToggleButton>
        <ToggleButton value="rounded">Rounded</ToggleButton>
        <ToggleButton value="pill">Pill</ToggleButton>
      </RadioGroupInput>
      <ColorInput
        label="Text color"
        defaultValue={buttonTextColor}
        onChange={(buttonTextColor) => updateData({ ...data, props: { ...data.props, buttonTextColor } })}
      />
      <ColorInput
        label="Button color"
        defaultValue={buttonBackgroundColor}
        onChange={(buttonBackgroundColor) => updateData({ ...data, props: { ...data.props, buttonBackgroundColor } })}
      />
      <MultiStylePropertyPanel
        names={['backgroundColor']}
        value={data.style}
        onChange={(style) => updateData({ ...data, style })}
      />
      <ColorInput
        label="Button border color"
        defaultValue={borderColor}
        onChange={(borderColor) => updateData({ ...data, props: { ...data.props, borderColor } })}
      />
      <SliderInput
        label="Button border size"
        iconLabel={<LineWeightOutlined sx={{ fontSize: 16 }} />}
        units="px"
        step={1}
        min={0}
        max={10}
        zeroLabel="None"
        defaultValue={borderSize}
        onChange={(borderSize) => updateData({ ...data, props: { ...data.props, borderSize } })}
      />
      <MultiStylePropertyPanel
        names={['fontFamily', 'fontSize', 'fontWeight', 'textAlign', 'padding']}
        value={data.style}
        onChange={(style) => updateData({ ...data, style })}
      />
    </BaseSidebarPanel>
  );
}

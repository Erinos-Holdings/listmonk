import React, { useState } from 'react';

import { TextProps, TextPropsSchema } from '@usewaypoint/block-text';

import BaseSidebarPanel from './helpers/BaseSidebarPanel';
import BooleanInput from './helpers/inputs/BooleanInput';
import MarkdownContentInput from './helpers/inputs/MarkdownContentInput';
import MultiStylePropertyPanel from './helpers/style-inputs/MultiStylePropertyPanel';

type TextSidebarPanelProps = {
  data: TextProps;
  setData: (v: TextProps) => void;
};
export default function TextSidebarPanel({ data, setData }: TextSidebarPanelProps) {
  const [, setErrors] = useState<Zod.ZodError | null>(null);

  const updateData = (d: unknown) => {
    const res = TextPropsSchema.safeParse(d);
    if (res.success) {
      setData(res.data);
      setErrors(null);
    } else {
      setErrors(res.error);
    }
  };

  return (
    <BaseSidebarPanel title="Text block">
      <MarkdownContentInput
        label="Content"
        rows={5}
        defaultValue={data.props?.text ?? ''}
        markdownEnabled={data.props?.markdown ?? false}
        onChange={(text) => updateData({ ...data, props: { ...data.props, text } })}
      />
      {/* Markdown is the default for new blocks and the toggle is hidden once
          on. It renders only for stored plain-text blocks as the escape hatch
          to opt in — flipping stored blocks automatically could reformat them
          (literal *, _, and newline handling change under markdown). */}
      {(data.props?.markdown ?? false) === false && (
        <BooleanInput
          label="Markdown"
          defaultValue={false}
          onChange={(markdown) => updateData({ ...data, props: { ...data.props, markdown } })}
        />
      )}

      {/* Font weight is retired from the panel (the ribbon's ** covers
          emphasis) but stored bold blocks keep their saved style — show the
          control for those so they aren't stranded bold with no way back. */}
      <MultiStylePropertyPanel
        names={
          data.style?.fontWeight === 'bold'
            ? ['fontFamily', 'fontSize', 'fontWeight', 'textAlign', 'padding', 'color', 'backgroundColor']
            : ['fontFamily', 'fontSize', 'textAlign', 'padding', 'color', 'backgroundColor']
        }
        value={data.style}
        onChange={(style) => updateData({ ...data, style })}
      />
    </BaseSidebarPanel>
  );
}

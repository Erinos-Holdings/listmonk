import React, { Fragment } from 'react';

import { TEditorBlock } from '../../../editor/core';
import EditorBlock, { useCurrentBlockId } from '../../../editor/EditorBlock';
import { setDocument, useDocument, useOfficialContext } from '../../../editor/EditorContext';
import { insertOfficialFooter } from '../../../../official/insert';

import AddBlockButton from './AddBlockMenu';

export type EditorChildrenChange = {
  blockId: string;
  block: TEditorBlock;
  childrenIds: string[];
};

function generateId() {
  return `block-${Date.now()}`;
}

export type EditorChildrenIdsProps = {
  childrenIds: string[] | null | undefined;
  onChange: (val: EditorChildrenChange) => void;
};
export default function EditorChildrenIds({ childrenIds, onChange }: EditorChildrenIdsProps) {
  // Fork (official footer) -- OFFICIAL-FOOTER-SPEC D7. The Add menu's "Official footer" entry
  // inserts the brand + corporate pair through the pure insertOfficialFooter transform (the
  // onSelect contract inserts one block). Offered only in the root layout or a Container (a
  // column is not a footer position), and hidden while an Official_ template is being edited
  // (context.official) -- a reference can never hold an OfficialFooter (D4).
  const parentId = useCurrentBlockId();
  const document = useDocument();
  const officialContext = useOfficialContext();
  const parentType = parentId ? document[parentId]?.type : undefined;
  const offerOfficial = !officialContext?.official && (parentType === 'EmailLayout' || parentType === 'Container');
  const insertOfficial = (index: number) => {
    const next = insertOfficialFooter(document, parentId, index, officialContext);
    if (next !== document) {
      setDocument(next as typeof document);
    }
  };
  const officialAt = (index: number) => (offerOfficial ? () => insertOfficial(index) : undefined);

  const appendBlock = (block: TEditorBlock) => {
    const blockId = generateId();
    return onChange({
      blockId,
      block,
      childrenIds: [...(childrenIds || []), blockId],
    });
  };

  const insertBlock = (block: TEditorBlock, index: number) => {
    const blockId = generateId();
    const newChildrenIds = [...(childrenIds || [])];
    newChildrenIds.splice(index, 0, blockId);
    return onChange({
      blockId,
      block,
      childrenIds: newChildrenIds,
    });
  };

  if (!childrenIds || childrenIds.length === 0) {
    return <AddBlockButton placeholder onSelect={appendBlock} onSelectOfficial={officialAt(0)} />;
  }

  return (
    <>
      {childrenIds.map((childId, i) => (
        <Fragment key={childId}>
          <AddBlockButton first={i === 0} onSelect={(block) => insertBlock(block, i)} onSelectOfficial={officialAt(i)} />
          <EditorBlock id={childId} />
        </Fragment>
      ))}
      <AddBlockButton onSelect={appendBlock} onSelectOfficial={officialAt(childrenIds.length)} />
    </>
  );
}

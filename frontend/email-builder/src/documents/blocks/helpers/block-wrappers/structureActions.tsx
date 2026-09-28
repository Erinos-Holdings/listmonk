import React, { useMemo, useState } from 'react';

import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from '@mui/material';

import { TEditorConfiguration } from '../../../editor/core';
import { resetDocument, setSelectedBlockId, useDocument } from '../../../editor/EditorContext';
import { deleteSubtree, duplicateSubtree, styleSummary, unwrap, unwrapRefusalMessage, wrapperState } from '../../../structure';

// Fork (container structure) -- CONTAINER-NESTING-SPEC §2.1/§2.2. The key-removing actions
// (Delete, Unwrap) and Duplicate, shared by the TuneMenu and the Container panel's Alert. Every
// transform comes from structure.ts. Commit rule: the store's setDocument MERGES and can never
// remove a key, so every transform here is committed with resetDocument(next) followed by
// setSelectedBlockId(...). Confirms are an MUI Dialog (the builder runs in an iframe, so no
// window.confirm).

type TConfirm = {
  kind: 'unwrap' | 'delete';
  title: string;
  lines: string[];
  confirmLabel: string;
  onConfirm: () => void;
};

function commit(next: unknown, select: string | null) {
  resetDocument(next as TEditorConfiguration);
  setSelectedBlockId(select);
}

export function useStructureActions(blockId: string) {
  const document = useDocument();
  const [confirm, setConfirm] = useState<TConfirm | null>(null);
  const block = document[blockId];
  const isContainer = block?.type === 'Container';

  const unwrapCheck = useMemo(() => (isContainer ? unwrap(document, blockId) : null), [document, blockId, isContainer]);
  const unwrapRefusal = unwrapCheck && 'refused' in unwrapCheck ? unwrapRefusalMessage(unwrapCheck.refused) : null;

  const doUnwrap = () => {
    const res = unwrap(document, blockId);
    if ('refused' in res) {
      return;
    }
    commit(res.doc, res.firstChildId);
  };

  const requestUnwrap = () => {
    if (!isContainer || unwrapRefusal) {
      return;
    }
    // D4: an unstyled container unwraps at once; a styled one first lists what is lost.
    if (wrapperState(block)) {
      doUnwrap();
      return;
    }
    setConfirm({
      kind: 'unwrap',
      title: 'Unwrap this container?',
      lines: ['Its blocks stay where they are; the container and its styling are removed:', ...styleSummary(block).map((s) => `• ${s}`)],
      confirmLabel: 'Unwrap',
      onConfirm: doUnwrap,
    });
  };

  const doDelete = () => {
    const res = deleteSubtree(document, blockId);
    commit(res.doc, null);
  };

  const requestDelete = () => {
    const { removed } = deleteSubtree(document, blockId);
    const type = block?.type;
    // D5: deleting a container with anything inside confirms first.
    if ((type === 'Container' || type === 'ColumnsContainer') && removed >= 1) {
      const lines = [`Delete this container and the ${removed} block${removed === 1 ? '' : 's'} inside it?`];
      if (type === 'Container') {
        lines.push('Use Unwrap to keep them.');
      }
      setConfirm({ kind: 'delete', title: 'Delete container', lines, confirmLabel: 'Delete', onConfirm: doDelete });
      return;
    }
    doDelete();
  };

  const requestDuplicate = () => {
    const res = duplicateSubtree(document, blockId);
    if (res.newId) {
      commit(res.doc, res.newId);
    }
  };

  const close = () => setConfirm(null);
  const dialog = confirm ? (
    <Dialog open onClose={close} data-lm-confirm={confirm.kind} onClick={(ev) => ev.stopPropagation()}>
      <DialogTitle>{confirm.title}</DialogTitle>
      <DialogContent>
        {confirm.lines.map((l, i) => (
          <DialogContentText key={i}>{l}</DialogContentText>
        ))}
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>Cancel</Button>
        <Button
          color={confirm.kind === 'delete' ? 'error' : 'primary'}
          variant="contained"
          data-lm-confirm-ok={confirm.kind}
          onClick={() => {
            const run = confirm.onConfirm;
            setConfirm(null);
            run();
          }}
        >
          {confirm.confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  ) : null;

  return { isContainer, unwrapRefusal, requestUnwrap, requestDelete, requestDuplicate, dialog };
}

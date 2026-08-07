import { TEditorConfiguration } from '../../documents/editor/core';

const EMPTY_EMAIL_MESSAGE: TEditorConfiguration = {
  root: {
    type: 'EmailLayout',
    data: {
      backdropColor: '#F5F5F5',
      canvasColor: '#FFFFFF',
      textColor: '#262626',
      fontFamily: 'MODERN_SANS',
      // Newly created documents opt into Outlook compatibility; stored documents
      // load via resetDocument() and keep whatever they saved.
      outlook: true,
      childrenIds: [],
    },
  },
};

export default EMPTY_EMAIL_MESSAGE;

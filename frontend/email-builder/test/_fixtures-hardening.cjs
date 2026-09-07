// Shared fixtures for the CAMPAIGN-52-HARDENING suites (T1–T5): a canvas with an inline
// VML-path button, a full-width button, an over-wide image, and text blocks on the layout
// default stack vs an explicit Arial stack. Same decoder as vml-button.test.cjs.
const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const outlook = require(path.join(__dirname, '.build', 'outlook.cjs'));

const MODERN_SANS = '&quot;Helvetica Neue&quot;, &quot;Arial Nova&quot;, &quot;Nimbus Sans&quot;, Arial, sans-serif';
const ARIAL = 'Arial, &quot;Helvetica Neue&quot;, Helvetica, sans-serif';

function canvas(inner, { backdropFont = MODERN_SANS, canvasStyle = 'margin:0 auto;max-width:600px;background-color:#fff' } = {}) {
  return `<!doctype html><html><head></head><body>
<div style="background-color:#eee;color:#262626;font-family:${backdropFont};font-size:16px;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="${canvasStyle}"><tbody><tr><td>
${inner}
</td></tr></tbody></table>
</div>
</body></html>`;
}

const inlineButton = `<div style="text-align:center;padding:0px 24px 20px 24px"><a href="https://x.test/go" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#000000;border-radius:64px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none;border:2px solid #fbf00b">Inline CTA</a></div>`;
const fullWidthButton = `<div style="text-align:center;padding:0px 24px 8px 24px"><a href="https://x.test/wide" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none;width:100%">Wide CTA</a></div>`;
const wideImage = `<div style="padding:0px 24px 0px 24px;text-align:center"><img alt="photo" src="https://x.test/photo.png" width="700" style="width:700px;max-width:100%;height:auto"></div>`;
const modernText = `<div style="font-weight:normal;text-align:center;padding:8px 24px 4px 24px"><p>Ciao {{ .Subscriber.FirstName }},</p></div>`;
const arialText = `<div style="font-family:${ARIAL};font-size:14px;padding:0px 24px 0px 24px"><p>Le ricompense sono limitate.</p></div>`;
// Markdown OFF: the vendored block-text emits the text as a bare text node, no <p>.
const plainModernText = `<div style="font-size:16px;font-weight:normal;padding:0px 24px 0px 24px">Tutto a metà prezzo.</div>`;
const spacerDiv = `<div style="height:8px;line-height:8px;font-size:8px">&nbsp;</div>`;
const rhythmModernText = `<div style="padding:0px 24px 0px 24px"><p>Hai sbloccato il <strong>50%</strong>.</p></div>`;
const borderedContainer = `<div style="border:1px solid #fbf00b;border-radius:0;padding:0px 24px 0px 24px"><div style="font-size:22px;font-weight:normal;padding:0px 0px 0px 0px"><p><strong>Scade il 15 ottobre</strong></p></div></div>`;

// Simulated Go render: Safe payloads decode and the href marker becomes its value.
function decodeSafe(out) {
  return out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
    s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\x20/g, ' ').replace(/\\x09/g, '\t').replace(/\\x26/g, '&').replace(/\\"/g, '"').replace(/\\\\/g, '\\'))
    .replace(/<span data-lm-vml-href="([^"]*)"><\/span>/g, (_, v) => v.replace(/&quot;/g, '"').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&'));
}

function makeChecker() {
  let failed = 0;
  const check = (name, ok, detail) => { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); };
  const done = () => { console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS'); process.exit(failed ? 1 : 0); };
  return { check, done };
}

module.exports = {
  JSDOM, outlook, canvas, decodeSafe, makeChecker,
  MODERN_SANS, ARIAL, inlineButton, fullWidthButton, wideImage, modernText, arialText, rhythmModernText, borderedContainer, plainModernText, spacerDiv,
};

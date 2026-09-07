// CAMPAIGN-52-HARDENING T1 (I1, I2). T-Online strips the contents of every
// downlevel-revealed conditional comment, so compiled output must never contain one; the
// non-Word twin of every VML button and clamped image rides in an element carrying
// mso-hide:all (a block wrapper; for a table twin, the table AND each of its cells).
const { outlook, canvas, decodeSafe, makeChecker, JSDOM, inlineButton, fullWidthButton, wideImage } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const out = outlook.postProcessForOutlook(canvas(inlineButton + fullWidthButton + wideImage));
const rendered = decodeSafe(out);

// I1 — nowhere, in either the stored (Safe-encoded) or the rendered form.
check('I1 stored body carries no downlevel-revealed opener', !out.includes('!mso'));
check('I1 rendered body carries no <!--[if !mso]>', !rendered.includes('<!--[if !mso]>'));
check('I1 rendered body carries no <!--<![endif]-->', !rendered.includes('<!--<![endif]-->'));
check('mso twins still ride in downlevel-hidden conditionals', (rendered.match(/<!--\[if mso\]>/g) || []).length >= 3);

// I2 — every non-Word twin is wrapped in mso-hide:all.
const doc = new JSDOM(rendered).window.document;
const inlineAnchor = doc.querySelector('a[href="https://x.test/go"]');
check('inline button CSS anchor exists', !!inlineAnchor);
check('inline button twin wrapped in a block carrying mso-hide:all',
  inlineAnchor && inlineAnchor.parentElement.tagName === 'DIV' && /mso-hide:\s*all/.test(inlineAnchor.parentElement.getAttribute('style') || ''));

const wideAnchor = doc.querySelector('a[href="https://x.test/wide"]');
check('full-width button CSS anchor exists', !!wideAnchor);
const wideTable = wideAnchor && wideAnchor.closest('table');
check('full-width twin table carries mso-hide:all', wideTable && /mso-hide:\s*all/.test(wideTable.getAttribute('style') || ''));
check('full-width twin table carries the lm-nomso class', wideTable && /\blm-nomso\b/.test(wideTable.getAttribute('class') || ''));
const wideCells = wideTable ? Array.from(wideTable.querySelectorAll('td')) : [];
check('every cell of the full-width twin carries mso-hide:all', wideCells.length > 0 && wideCells.every((td) => /mso-hide:\s*all/.test(td.getAttribute('style') || '')));
check('full-width twin keeps width=100% (fluid for Outlook mobile)', wideTable && wideTable.getAttribute('width') === '100%');

// The clamped Word copy lives inside a downlevel-hidden conditional — a COMMENT to the
// DOM — so it is asserted on the rendered string; the original is a real element.
const imgTags = rendered.match(/<img[^>]*src="https:\/\/x\.test\/photo\.png"[^>]*>/g) || [];
check('over-wide image is dual-emitted (clamped mso copy + original)', imgTags.length === 2, `imgs=${imgTags.length}`);
check('clamped copy rides inside <!--[if mso]> … <![endif]-->', /<!--\[if mso\]><img[^>]*width="552"[^>]*><!\[endif\]-->/.test(rendered));
const original = doc.querySelector('img[src="https://x.test/photo.png"]');
check('original image twin (width 700) is the one real element, wrapped in a block carrying mso-hide:all',
  original && original.getAttribute('width') === '700' && original.parentElement.tagName === 'DIV' && /mso-hide:\s*all/.test(original.parentElement.getAttribute('style') || ''));

done();

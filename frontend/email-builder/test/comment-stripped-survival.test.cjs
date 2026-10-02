// CAMPAIGN-52-HARDENING T2 (I3). A T-Online-style sanitizer removes every <!--…--> span
// wholesale; the document must still carry both button anchors and the image afterwards.
const { pp, canvas, decodeSafe, makeChecker, JSDOM, inlineButton, fullWidthButton, fallbackButton, wideImage, headingBlock, twoColumnRow } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const rendered = decodeSafe(pp.postProcess(canvas(inlineButton + fullWidthButton + fallbackButton + wideImage + headingBlock + twoColumnRow), { outlook: true }));
const stripped = rendered.replace(/<!--[\s\S]*?-->/g, '');
check('sanitizer removed the Word twins', !stripped.includes('v:roundrect'));

const doc = new JSDOM(stripped).window.document;
const anchors = Array.from(doc.querySelectorAll('a')).filter((a) => /Inline CTA|Wide CTA/.test(a.textContent));
check('both button anchors survive with their text', anchors.length === 2 && anchors.some((a) => a.textContent.trim() === 'Inline CTA') && anchors.some((a) => a.textContent.trim() === 'Wide CTA'), `anchors=${anchors.length}`);
check('inline anchor keeps its href', !!doc.querySelector('a[href="https://x.test/go"]'));
check('full-width anchor keeps its href', !!doc.querySelector('a[href="https://x.test/wide"]'));
// BIBLE-OUTLOOK-FIXES-SPEC §12 (IA15): the fallback Button's Word table is gone and its non-Word
// twin survives with its label and href.
check('the fallback\'s Word table was stripped', !/data-lm-btn-fallback/.test(stripped));
const fb = Array.from(doc.querySelectorAll('a[href="https://x.test/fallback"]'));
check('the fallback Button\'s anchor survives with its text', fb.length === 1 && fb[0].textContent.trim() === 'Fallback CTA →', `anchors=${fb.length}`);
const imgs = doc.querySelectorAll('img[src="https://x.test/photo.png"]');
check('exactly one image with the src survives (the clamped copy, 2026-09-25: no unclamped twin)', imgs.length === 1 && imgs[0].getAttribute('width') === '552', `imgs=${imgs.length}`);

// BIBLE-OUTLOOK-FIXES-SPEC I8: a Heading block (now a table cell with a Word font wrapper inside
// it) and a two-column row (lm-cw- classes, a Word-only style payload in <head>) survive too.
check('the Word font wrapper and the column style block were stripped', !/<font face=|td\.lm-cw-/.test(stripped));
const h2 = doc.querySelector('h2');
check('the heading survives with its text, inside its cell', !!h2 && h2.textContent.trim() === 'Heading survives' && h2.parentElement.tagName === 'TD');
const cols = Array.from(doc.querySelectorAll('td[class*="lm-cw-"]'));
check('both columns survive with their content', cols.length === 2 && /Left column survives/.test(cols[0].textContent) && /Right column survives/.test(cols[1].textContent), `cols=${cols.length}`);

done();

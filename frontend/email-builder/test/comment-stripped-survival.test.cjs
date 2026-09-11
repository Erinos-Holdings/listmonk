// CAMPAIGN-52-HARDENING T2 (I3). A T-Online-style sanitizer removes every <!--…--> span
// wholesale; the document must still carry both button anchors and the image afterwards.
const { pp, canvas, decodeSafe, makeChecker, JSDOM, inlineButton, fullWidthButton, wideImage } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const rendered = decodeSafe(pp.postProcess(canvas(inlineButton + fullWidthButton + wideImage), { outlook: true }));
const stripped = rendered.replace(/<!--[\s\S]*?-->/g, '');
check('sanitizer removed the Word twins', !stripped.includes('v:roundrect'));

const doc = new JSDOM(stripped).window.document;
const anchors = Array.from(doc.querySelectorAll('a')).filter((a) => /Inline CTA|Wide CTA/.test(a.textContent));
check('both button anchors survive with their text', anchors.length === 2 && anchors.some((a) => a.textContent.trim() === 'Inline CTA') && anchors.some((a) => a.textContent.trim() === 'Wide CTA'), `anchors=${anchors.length}`);
check('inline anchor keeps its href', !!doc.querySelector('a[href="https://x.test/go"]'));
check('full-width anchor keeps its href', !!doc.querySelector('a[href="https://x.test/wide"]'));
const imgs = doc.querySelectorAll('img[src="https://x.test/photo.png"]');
check('exactly one image with the src survives (the original, not the clamped copy)', imgs.length === 1 && imgs[0].getAttribute('width') === '700', `imgs=${imgs.length}`);

done();

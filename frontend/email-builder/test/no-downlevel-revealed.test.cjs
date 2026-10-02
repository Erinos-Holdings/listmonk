// CAMPAIGN-52-HARDENING T1 (I1, I2). T-Online strips the contents of every
// downlevel-revealed conditional comment, so compiled output must never contain one; the
// non-Word twin of every VML button and clamped image rides in an element carrying
// mso-hide:all (a block wrapper; for a table twin, the table AND each of its cells).
const { pp, canvas, decodeSafe, makeChecker, JSDOM, inlineButton, fullWidthButton, fallbackButton, wideImage, headingBlock, twoColumnRow } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const out = pp.postProcess(canvas(inlineButton + fullWidthButton + fallbackButton + wideImage + headingBlock + twoColumnRow), { outlook: true });
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
// BIBLE-OUTLOOK-FIXES-SPEC §14 (SC1): Word ignored the per-cell declaration and painted the
// twin beneath the VML copy (Outlook 2024, bible sheet 5, 2026-10-02); the twin table is also
// wrapped in the block div Word honours for the inline Button. Fails with the fix reverted.
check('full-width twin table is wrapped in a block div carrying mso-hide:all and lm-nomso',
  wideTable && wideTable.parentElement.tagName === 'DIV' && /mso-hide:\s*all/.test(wideTable.parentElement.getAttribute('style') || '') && /\blm-nomso\b/.test(wideTable.parentElement.getAttribute('class') || ''));
check('the wrapper holds the twin table alone', wideTable && wideTable.parentElement.children.length === 1);

// BIBLE-OUTLOOK-FIXES-SPEC §12 (IA15): the fallback Button's Word copy (the stamped table cell)
// rides in a downlevel-HIDDEN conditional, and its non-Word twin in an mso-hide:all block.
check('the fallback table is a downlevel-hidden conditional', /<!--\[if mso\]><table [^>]*data-lm-btn-fallback="chars"[\s\S]*?<\/table><!\[endif\]-->/.test(rendered));
const fallbackAnchors = Array.from(doc.querySelectorAll('a[href="https://x.test/fallback"]'));
check('the fallback\'s CSS anchor is wrapped in a block carrying mso-hide:all',
  fallbackAnchors.length === 1 && fallbackAnchors[0].parentElement.tagName === 'DIV' && /mso-hide:\s*all/.test(fallbackAnchors[0].parentElement.getAttribute('style') || ''), `anchors=${fallbackAnchors.length}`);

// 2026-09-25: an over-wide image is emitted ONCE, clamped, as a real element for every
// client — no Word conditional and no mso-hide twin (the unclamped twin made the Gmail
// apps overflow a Columns row).
const imgTags = rendered.match(/<img[^>]*src="https:\/\/x\.test\/photo\.png"[^>]*>/g) || [];
check('over-wide image is emitted once', imgTags.length === 1, `imgs=${imgTags.length}`);
check('no image rides inside <!--[if mso]> … <![endif]-->', !/<!--\[if mso\]><img/.test(rendered));
const clampedImg = doc.querySelector('img[src="https://x.test/photo.png"]');
check('the one real image is clamped to 552 and not wrapped in an mso-hide:all block',
  clampedImg && clampedImg.getAttribute('width') === '552' && !(clampedImg.parentElement.tagName === 'DIV' && /mso-hide:\s*all/.test(clampedImg.parentElement.getAttribute('style') || '')));

// BIBLE-OUTLOOK-FIXES-SPEC I8: the Heading cell's font wrapper and the column-width style block
// are Word-only and ride in downlevel-HIDDEN conditionals (checked by I1 above for the whole
// body); the content they decorate is ordinary markup every client reads.
check('the heading font wrapper is a downlevel-hidden conditional', /<h2 [^>]*><!--\[if mso\]><font face="Arial"><!\[endif\]-->Heading survives<!--\[if mso\]><\/font><!\[endif\]--><\/h2>/.test(rendered));
check('the column style block is a downlevel-hidden conditional in <head>', /<head>[\s\S]*<!--\[if mso\]><style>td\.lm-cw-268\{width:268px\}<\/style><!\[endif\]-->[\s\S]*<\/head>/.test(rendered));
check('the heading and both columns are real elements', !!doc.querySelector('h2') && /Heading survives/.test(doc.querySelector('h2').textContent)
  && doc.querySelectorAll('td.lm-cw-268').length === 2 && /Left column survives/.test(doc.querySelectorAll('td.lm-cw-268')[0].textContent) && /Right column survives/.test(doc.querySelectorAll('td.lm-cw-268')[1].textContent));

done();

// CAMPAIGN-52-HARDENING T3 (I4). The VML <center> label carries the dark-mode-proof shape of
// whichever D2 variant outlook.ts ships (VML_LABEL_VARIANT); the canonical anchorlock+center
// shape and the no-textbox pin hold under every variant. Behavior (Word's dark transform)
// is external-client rendering — gate G2, not this suite.
const { outlook, canvas, decodeSafe, makeChecker, inlineButton } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const variant = outlook.VML_LABEL_VARIANT;
check('VML_LABEL_VARIANT is one of the D2 candidates', ['font', 'bgcolor', 'border'].includes(variant), String(variant));

const rendered = decodeSafe(outlook.postProcessForOutlook(canvas(inlineButton)));
const vml = (rendered.match(/<v:roundrect[\s\S]*?<\/v:roundrect>/) || [])[0] || '';
check('VML roundrect rendered', vml.length > 0);
check('canonical: anchorlock then center, no textbox', /<w:anchorlock\/><center/.test(vml) && !/v:textbox/.test(vml));
check('fill stays the button color (light-mode design unchanged)', /fillcolor="#000000"/.test(vml));
check('stroke is the border color', /strokecolor="#fbf00b"/.test(vml));

const center = (vml.match(/<center[\s\S]*?<\/center>/) || [])[0] || '';
switch (variant) {
  case 'font':
    check('(a) label wrapped in <font color> + <span style=color> inside the center',
      /<center style="color:#FFFFFF;[^"]*"><font color="#FFFFFF"><span style="color:#FFFFFF">Inline CTA<\/span><\/font><\/center>/.test(center), center);
    check('(a) no background on the center', !/background/.test(center));
    break;
  case 'bgcolor':
    check('(b) dark background on the center itself, label color unchanged',
      /<center style="color:#FFFFFF;[^"]*background:#000000;[^"]*">Inline CTA<\/center>/.test(center), center);
    check('(b) wrapper td carries no bgcolor/background (would paint a band in light mode)',
      !/<td[^>]*bgcolor="#000000"/.test(rendered) && !/<td[^>]*style="[^"]*background(?:-color)?:#000000/.test(rendered));
    break;
  case 'border':
    check('(c) label in the border color', /<center style="color:#fbf00b;/.test(center), center);
    break;
  default:
}

done();

# Run from this folder: python3 build.py. Edit proto2-template.html, never the built page.
# Builds prototype v2 (variant A, live): kit CSS and icons from the prototype, shared lab CSS and reusable script blocks from lab 2, the three-exchange snapshot.
S = '../'   # the built page goes one level up, next to this folder
tpl = open('proto2-template.html').read()
v2 = open('lab2-template.html').read()
proto = open('screener-v1.html').read()
def cut(src, a, b, incl=False):
    i = src.index(a); j = src.index(b, i)
    return src[i:j + (len(b) if incl else 0)]
css = proto[proto.index('<style>') + 7:proto.index('</style>')]; css = css[css.index(':root {'):].rstrip()
i0 = proto.index('<svg width="0" height="0"'); i1 = proto.index('</svg>', proto.index('id="i-gear"')) + 6
lab2css = cut(v2, '/* ===== Lab additions', '</style>').rstrip()
fmt = cut(v2, 'function fmtPrice', '// Sparkline of the prototype').rstrip()
x, y = "v >= 0.01 ? 5 : 7;", "v >= 0.01 ? 5 : v >= 0.0001 ? 7 : 10;"
assert x in fmt; fmt = fmt.replace(x, y)
levels = cut(v2, '// Hourly bars since the level came into being', 'function passes(').rstrip()
cols = cut(v2, 'const colLabel', 'F.cols = F.cols.filter(k => COL[k])').rstrip()
catalog = cut(v2, '// The catalogue a newcomer reads', 'const SUGGEST').rstrip()
for x, y in (("{ name: 'Mark price', desc: 'Current price in USD'", "{ name: 'Price', desc: 'Last price in USD'"), ("desc: 'What longs pay shorts per interval'", "desc: 'What longs pay shorts, brought to 8 hours'"), ("desc: 'Traded over the last day, in USD'", "desc: 'Traded over the last day on the selected exchanges, in USD'"), ("desc: 'Value of all open positions, in USD'", "desc: 'Open positions on the selected exchanges, in USD'")):
    assert x in catalog, x; catalog = catalog.replace(x, y)
drag = cut(v2, 'function makeDrag(', "makeDrag($('thead')").rstrip()
a, b = 'function makeDrag(root, selector, axis, canDrag) {', "document.body.classList.add('-dragging'); closePop();"
assert a in drag and b in drag
drag = drag.replace(a, 'function makeDrag(root, selector, axis, canDrag, keepPop) {').replace(b, "document.body.classList.add('-dragging'); if (!keepPop) closePop();")
out = tpl.replace('__KIT_CSS__', css).replace('__LAB2_CSS__', lab2css).replace('__ICONS__', proto[i0:i1])
for k, v in (('/*@FMT@*/', fmt), ('/*@LEVELS@*/', levels), ('/*@COLS@*/', cols), ('/*@CATALOG@*/', catalog), ('/*@DRAG@*/', drag)):
    assert k in out; out = out.replace(k, v)
snap = open('proto2-snapshot.json').read(); assert '</' not in snap
out = out.replace('__SNAPSHOT__', snap)
open(S + 'screener-v2.html', 'w').write(out)
# artifact copy: the publisher adds its own doctype, charset and viewport
lines = out.split('\n'); assert lines[0].startswith('<!doctype') and 'charset' in lines[1] and 'viewport' in lines[2]
print('built', len(out), 'bytes')

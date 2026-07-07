// utils/icons.ts — Font Awesome Pro fallback.
//
// Tracker defs use the EXACT icon classes the tracker sites use, many of
// which are FA Pro-only (fa-lobster, fa-squirrel, fa-whale, …). Users who
// drop Font Awesome Pro into static/fontawesome/ see the real icons; for
// everyone else those classes have no CSS rule and render as blank space.
//
// This module detects missing glyphs at runtime (an icon class with no
// matching stylesheet rule has no ::before content) and swaps them for a
// free fallback icon. A MutationObserver covers every render path — grid,
// table, modals, popovers — with no per-render wiring. Start it ONLY after
// the stylesheet situation is settled (free CDN loaded, self-hosted Pro
// probe finished), or Pro icons would be "fixed" before their CSS arrives.

/** Free icon substituted for any icon class that has no glyph. */
const FALLBACK = 'fas fa-star';

/** Amber globe marking an active tracker event (freeleech/announcement) —
 *  echoes the 🌐 most trackers use for "global" events. Inline SVG (not an
 *  FA class) so it renders regardless of icon-set state, stroked with
 *  var(--amber) so it follows themes. Replaced the old bell, which now
 *  exclusively means "unread notifications". */
export function eventGlobeSvg(extraStyle = ''): string {
  return `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="var(--amber)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"${extraStyle ? ` style="${extraStyle}"` : ''}><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>`;
}

/** class string → has a renderable glyph (cached — one probe per class). */
const glyphCache = new Map<string, boolean>();

let probeBox: HTMLElement | null = null;
let started = false;

function ensureProbeBox(): HTMLElement {
  if (!probeBox) {
    probeBox = document.createElement('div');
    // Rendered but invisible — display:none would suppress ::before styles.
    probeBox.style.cssText =
      'position:absolute;left:-9999px;top:-9999px;visibility:hidden;pointer-events:none';
    document.body.appendChild(probeBox);
  }
  return probeBox;
}

/** True when the class string produces an actual ::before glyph.
 *  Handles both FA mechanisms: direct `content:"\fxxx"` rules (FA5-style)
 *  and FA6's `--fa` custom property consumed by `.fas::before{content:
 *  var(--fa)}`. A missing icon has neither: computed content is "none"
 *  (var() invalid at computed-value time) and --fa is unset. */
function hasGlyph(cls: string): boolean {
  const cached = glyphCache.get(cls);
  if (cached !== undefined) return cached;
  const el = document.createElement('i');
  el.className = cls;
  ensureProbeBox().appendChild(el);
  const content = getComputedStyle(el, '::before').content;
  const fa = getComputedStyle(el).getPropertyValue('--fa').trim();
  el.remove();
  const ok = (content !== 'none' && content !== 'normal') || fa !== '';
  glyphCache.set(cls, ok);
  return ok;
}

/** Swap glyph-less fa-* icons under root for the free fallback icon. */
export function fixupIcons(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>('i[class*="fa-"]').forEach(el => {
    if (el.dataset['faFallback']) return;
    const cls = el.className;
    if (!cls || hasGlyph(cls)) return;
    el.className = FALLBACK;
    el.dataset['faFallback'] = '1'; // don't re-probe; keeps inline color/style
  });
}

/**
 * Start the fallback engine: one full-document sweep now, then a
 * MutationObserver keeps fixing icons as the app re-renders.
 * Call AFTER the final icon stylesheet (free or self-hosted Pro) has loaded.
 */
export function startIconFallback(): void {
  if (started) return;
  started = true;
  fixupIcons(document);
  const mo = new MutationObserver(muts => {
    for (const m of muts) {
      m.addedNodes.forEach(n => {
        if (n.nodeType === Node.ELEMENT_NODE) fixupIcons(n as Element);
      });
    }
  });
  mo.observe(document.body, { childList: true, subtree: true });
}

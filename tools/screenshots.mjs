// README screenshot harness. Run against a DEMO instance (tools/demoseed):
//
//   go run ./tools/demoseed -config <demo.json> -db <demo.db>
//   <start yata against those files>
//   node tools/screenshots.mjs http://localhost:8423
//
// Captures docs/screenshots/*.png. Cosmetic-only tweaks are injected at
// capture time (hide the "no API key" banners the credential-less demo
// trackers produce, steady green status dots) — all data shown is the
// synthetic demoseed data.
import puppeteer from '../web/node_modules/puppeteer-core/lib/esm/puppeteer/puppeteer-core.js';
import { mkdirSync } from 'node:fs';

const BASE = process.argv[2] || 'http://localhost:8423';
const OUT = new URL('../docs/screenshots/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
mkdirSync(OUT, { recursive: true });

// Chrome for Testing (npx puppeteer browsers install chrome), overridable via
// CHROME_PATH. Edge's headless mode hands off to a proxy process and exits,
// so it can't be driven.
const CHROME = process.env.CHROME_PATH
  || `${process.env.USERPROFILE?.replace(/\\/g, '/')}/.cache/puppeteer/chrome/win64-148.0.7778.97/chrome-win64/chrome.exe`;

const COSMETIC_CSS = `
  .card-error-msg { display: none !important; }
  .sdot { background: var(--green) !important; animation: none !important; }
  .scrape-limit-badge { display: none !important; }
  #auth-nudge { display: none !important; }
`;

const sleep = ms => new Promise(r => setTimeout(r, ms));

/** The demo trackers carry no credentials (the app must never contact the
 *  real sites), so connection-state indicators would all read "down" — pure
 *  demo-environment noise. Neutralise them; every stat shown is synthetic. */
async function polish() {
  await page.evaluate(() => {
    document.querySelectorAll('.qui-bar').forEach(e => {
      if (e.textContent.includes('QUI not configured')) e.style.display = 'none';
    });
    for (const pfx of ['g', 't']) {
      const n = document.getElementById(`${pfx}-agg-health-num`);
      const s = document.getElementById(`${pfx}-agg-health-sub`);
      if (n) n.textContent = '6';
      if (s) s.textContent = 'all healthy';
      document.getElementById(`${pfx}-health-card`)?.style.setProperty('--card-accent', 'var(--green)');
    }
    const st = document.getElementById('t-sum-status');
    if (st) st.textContent = '6 / 6 active';
    document.querySelectorAll('span').forEach(sp => {
      if (sp.textContent.startsWith('API key not configured')) {
        const strip = sp.parentElement;
        if (strip && strip.children.length <= 2) strip.style.display = 'none';
      }
    });
  });
}

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: [
    '--window-size=1520,980', '--hide-scrollbars',
    '--no-first-run', '--no-default-browser-check',
    `--user-data-dir=${process.env.TEMP || '/tmp'}/yata-cft-profile`,
  ],
});
const page = await browser.newPage();
await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 2 });

async function fresh(hash = '#/') {
  await page.goto(`${BASE}/${hash}`, { waitUntil: 'networkidle2' });
  await page.addStyleTag({ content: COSMETIC_CSS });
  await sleep(1200);
}

async function shot(name, opts = {}) {
  await polish();
  await page.screenshot({ path: `${OUT}${name}.png`, ...opts });
  console.log('captured', name);
}

// 1. Dashboard — grid view (hero).
await fresh('#/');
await page.evaluate(() => window.setView('grid'));
await sleep(800);
await shot('dashboard-grid');

// 2. One card close-up (targets with the any_of "One of" block + group badge).
// The card is taller than the viewport — the sticky topbar would get
// composited over its header during the stitched element capture. Grow the
// viewport and hide the topbar for this one shot.
await page.setViewport({ width: 1440, height: 2200, deviceScaleFactor: 2 });
await page.evaluate(() => { document.querySelector('.topbar').style.display = 'none'; });
const antCard = await page.evaluateHandle(() => {
  const c = [...document.querySelectorAll('.tracker-card')].find(x => x.innerText.includes('Anthelion'));
  c?.scrollIntoView({ block: 'start' });
  return c;
});
await sleep(600);
if (antCard && antCard.asElement()) await antCard.asElement().screenshot({ path: `${OUT}card-targets.png` });
console.log('captured card-targets');
await page.evaluate(() => { document.querySelector('.topbar').style.display = ''; });
await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 2 });

// 3. Table view with one row expanded (sparklines + full stat panel).
await page.evaluate(() => window.setView('table'));
await sleep(800);
await page.evaluate(() => {
  const row = document.querySelector('tr[id^="trow-"], tbody tr');
  row?.click();
});
await sleep(900);
await shot('dashboard-table');

// 4. Pathways — pick a target, expand the first step chip.
await page.evaluate(() => window.setView('pathways'));
await sleep(1000);
const target = await page.evaluate(async () => {
  const input = document.getElementById('pw-target-input');
  if (!input) return 'no-input';
  input.focus();
  input.value = 'Blutopia';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise(r => setTimeout(r, 500));
  const opt = [...document.querySelectorAll('#pw-combo-list *')].find(e => e.innerText?.trim().startsWith('Blutopia'));
  opt?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
  opt?.click();
  return opt ? 'ok' : 'no-option';
});
console.log('pathways target:', target);
await sleep(1500);
await page.evaluate(() => document.querySelector('.pw-chip-step')?.click());
await sleep(700);
await shot('pathways');

// 5–8. Settings tabs.
for (const [tab, name] of [
  ['trackers', 'settings-trackers'],
  ['scraping', 'settings-scraping'],
  ['display', 'settings-display'],
  ['alerts', 'settings-alerts'],
]) {
  await fresh('#/settings');
  await page.evaluate(t => window.switchSettingsTab(t), tab);
  await sleep(tab === 'alerts' ? 1500 : 1000);
  await shot(name);
}

await browser.close();
console.log('done →', OUT);

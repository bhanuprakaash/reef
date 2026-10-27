// Drives the Reef window in Chrome and prints PASS/FAIL lines. See ../../SKILL.md.
// usage: node drive.mjs live|brief|attach|cap|pages|stop|wrap|teams|retry|rollback|inbox|diff|memory|inspector|review|board|workshop|capture|theme|phone|fixes|pr|coordination|team|denials|subagent|composer|naming|hovercard|mention|providers|first-run BASE_URL
// env: REEF_UI_DATA (data folder of the server, for sqlite3), REEF_UI_PROJECT (project
// folder, default a new temp dir), REEF_UI_PROOF (screenshot folder, default ./proof),
// REEF_VERIFY_CHROME (path of a Chrome or Chromium binary; default the system Chrome),
// REEF_FAKE_GH (pr only: the folder the server's fake gh keeps its PR in, see fake-gh/gh).
// REEF_FAKE_LOG (denials and naming only: the fake claude's log, one line per turn with its args).
// REEF_UI_REEF (memory only: the reef binary, to run reef memory add).
// REEF_UI_FAKE (providers only: the folder holding the fake claude, used as a second install).
import { chromium } from 'playwright-core';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import path from 'node:path';

const [scenario, base] = process.argv.slice(2);
const scenarios = { live, brief, attach, cap, pages, stop, wrap, teams, retry, rollback, inbox, diff, memory, inspector, review, workshop, capture, theme, phone, pr, coordination, team, denials, subagent, composer, naming, hovercard, mention, providers, 'first-run': firstRun };
if (!scenarios[scenario] || !base) {
  console.error('usage: node drive.mjs live|brief|attach|cap|pages|stop|wrap|teams|retry|rollback|inbox|diff|memory|inspector|review|board|workshop|capture|theme|phone|fixes|pr|coordination|team|denials|subagent|composer|naming|hovercard|mention|providers|first-run BASE_URL');
  process.exit(2);
}
const data = process.env.REEF_UI_DATA;
if (!data) {
  console.error('set REEF_UI_DATA to the data folder of the server');
  process.exit(2);
}
const proof = path.resolve(process.env.REEF_UI_PROOF ?? 'proof');
const projectDir = process.env.REEF_UI_PROJECT ?? mkdtempSync(path.join(tmpdir(), 'reef-ui-'));
mkdirSync(proof, { recursive: true });

const sql = (q, write = false) =>
  execFileSync('sqlite3', ['-cmd', '.timeout 5000', ...(write ? [] : ['-readonly']), path.join(data, 'reef.db'), q], {
    encoding: 'utf8',
  }).trim();

let failed = false;
function check(ok, label, detail) {
  console.log(`${ok ? 'PASS' : 'FAIL'} ${scenario}: ${label}${detail ? ` | ${detail}` : ''}`);
  if (!ok) failed = true;
}

const chrome = process.env.REEF_VERIFY_CHROME;
const browser = await chromium.launch(
  chrome ? { executablePath: chrome, args: ['--no-sandbox'], headless: true } : { channel: 'chrome', headless: true },
);
const page = await browser.newPage({ viewport: { width: 1200, height: 1000 } });
const main = page.getByRole('main');
const aside = page.locator('aside:not([aria-label])');
const shot = async (step) => {
  const file = path.join(proof, `${scenario}-${step}.png`);
  await page.screenshot({ path: file });
  console.log(`shot ${file}`);
};

// Opens the custom select named `label` (inside `within`) and clicks the option named `option`.
async function pick(label, option, within = page) {
  await within.getByRole('combobox', { name: label, exact: true }).click();
  await page.getByRole('listbox', { name: label, exact: true }).getByRole('option', { name: option, exact: true }).click();
}

async function send(text) {
  await page.getByLabel('Message').fill(text);
  await page.getByLabel('Message').press('Enter');
}
async function working() {
  await page.getByText('is working').waitFor({ timeout: 30000 });
}
async function idle() {
  await page.getByText('is working').waitFor({ state: 'hidden', timeout: 240000 });
  await page.getByRole('button', { name: 'Stop' }).waitFor({ state: 'hidden' });
}
async function exchange(text) {
  await send(text + ' Answer in one or two sentences, use no tools.');
  await working();
  await idle();
}

async function submitProject(name, folder) {
  const switcher = aside.getByRole('button', { name: 'Switch project' });
  await page.getByLabel('Your name').or(page.getByLabel('Repo folder')).or(switcher).waitFor();
  if (await page.getByLabel('Your name').count()) {
    await page.getByLabel('Your name').fill('Tester');
    await page.getByRole('button', { name: 'Continue' }).click();
  }
  await page.getByLabel('Repo folder').or(switcher).waitFor();
  if (await page.getByLabel('Repo folder').count()) {
    await page.getByLabel('Repo folder').fill(folder);
    await page.getByLabel('Name', { exact: true }).fill(name);
    await page.getByRole('button', { name: 'Add project' }).click();
    return;
  }
  await switcher.click();
  await page.getByRole('menuitem', { name: 'New project…' }).click();
  await page.getByLabel('Repo folder').fill(folder);
  await page.getByLabel('Name', { exact: true }).fill(name);
  await page.getByRole('button', { name: 'Add project' }).click();
}

// Adds a project through the form; returns its sidebar section.
async function addProject() {
  const name = `ui-${scenario}-${Date.now().toString(36)}`;
  // A Space of a feature runs in a git worktree of the project, so the folder needs a commit.
  const git = (...args) => execFileSync('git', ['-C', projectDir, '-c', 'user.name=reef', '-c', 'user.email=reef@example.com', ...args], { encoding: 'utf8', stdio: 'pipe' });
  try {
    git('rev-parse', '--verify', 'HEAD');
  } catch {
    git('init', '-q');
    git('commit', '-q', '--allow-empty', '-m', 'init');
  }
  await page.goto(base + '/');
  await submitProject(name, projectDir);
  const section = aside.getByRole('region', { name });
  await section.waitFor();
  return section;
}
// Adds a work item and returns its id, which names its worktree folder and branch.
async function addFeature(section, name) {
  await section.getByRole('button', { name: 'New feature' }).click();
  await section.getByLabel('Feature title').fill(name);
  await section.getByRole('button', { name: 'Add', exact: true }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return page.url().split('/').pop();
}
// Starts a claude Space from the form in `box` and returns its id.
async function newSpace(box, openName, name, mode = 'acceptEdits') {
  await box.getByRole('button', { name: openName }).click();
  await pick('Backend', 'claude', box);
  if (mode) await pick('Permission mode', mode, box);
  await box.getByLabel('Name', { exact: true }).fill(name);
  await box.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return sql(`select id from Space where name='${name}' order by id desc limit 1`);
}

// The label of the running turn names its role: `<role> is working`.
const liveLabel = main.getByText('is working').locator('..');
async function workingAs(role, timeout = 240000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if ((await liveLabel.count()) > 0) {
      const text = (await liveLabel.first().textContent().catch(() => '')) ?? '';
      if (text.replace('is working', '').trim() === role) return true;
    }
    await page.waitForTimeout(250);
  }
  return false;
}
// The running turn is one compact row in the Space; clicking it opens the panel with the full stream.
const turnPanel = page.getByRole('complementary', { name: 'Turn' });
async function openTurn() {
  await main.getByRole('button', { name: /is working/ }).click();
  await turnPanel.waitFor();
}
// The top-level message rows of the main list that `author` wrote (they have the Reply button).
const rootRow = (author) =>
  main.locator('ul > li:has(> button)').filter({ has: page.getByText(author, { exact: true }) });
// Starts a Space on `team` from the form (backend and permission mode stay the team's).
async function newTeamSpace(team, name, step) {
  await main.getByRole('button', { name: 'New Space' }).click();
  const card = page.getByRole('dialog', { name: 'New Space' }).getByRole('button', { name: team, exact: true });
  await card.waitFor();
  const options = await card.locator('xpath=..').getByRole('button').allTextContents();
  await card.click();
  await main.getByLabel('Name', { exact: true }).fill(name);
  if (step) await shot(step);
  await main.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return { id: page.url().split('/').pop(), options };
}
const getJson = async (route) => (await fetch(base + route)).json();

async function live() {
  writeFileSync(path.join(projectDir, 'notes.txt'), 'Reef window check: notes file.\n');
  writeFileSync(path.join(projectDir, 'todo.txt'), 'Reef window check: todo file.\n');
  const section = await addProject();
  await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'live chat');
  await send(
    'List the files in this directory with a tool, then read one of them with a tool, then summarize what you saw in two sentences.',
  );
  await working();
  const row = main.getByRole('button', { name: /is working/ });
  const card = row.locator('xpath=..');
  await card.locator('details summary').first().waitFor({ timeout: 120000 });
  await page.waitForTimeout(1000);
  await shot('midturn');
  const inline = await card.locator('details summary').count();
  const first = await card.locator('details summary').first().innerText();
  check(inline >= 1 && inline <= 3 && /(Bash|Read|Glob|Grep|LS|List)\b/.test(first), 'the running card shows the last tool calls inline, at most 3', `${inline} rows, first: ${first}`);
  await openTurn();
  await turnPanel.locator('details summary').first().waitFor({ timeout: 120000 });
  await shot('turn-panel');
  const rows = await turnPanel.locator('details summary').count();
  check(rows >= 1, 'tool rows shown in the turn panel while the turn runs', `${rows} rows, first: ${await turnPanel.locator('details summary').first().innerText()}`);
  await idle();
  await shot('final');
  const lead = sql(`select count(*) from message where space_id=${cid} and author='lead'`);
  check(lead === '1', 'reply stored as one lead message', `message rows:\n${sql(`select id,author,substr(body,1,100) from message where space_id=${cid}`)}`);
  check((await main.locator('details').count()) === 0, 'live view cleared after the turn');
}

async function brief() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const featureId = page.url().split('/').pop();
  const res = await fetch(`${base}/work-items/${featureId}/brief`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ body: 'Goal: build the login page with magic links.', author: 'user' }),
  });
  check(res.status === 201, 'brief stored over REST', `PUT brief -> ${res.status}`);
  await page.reload();
  const cid = await newSpace(main, 'New Space', 'brief check', '');
  await send('what is the goal of this feature?');
  await working();
  await idle();
  await shot('answer');
  const answer = sql(`select body from message where author='lead' and space_id=${cid}`);
  console.log(`answer: ${answer}`);
  check(/magic/i.test(answer), 'a new Space answers from the brief', 'answer mentions magic links');
}

async function attach() {
  ensureRepo();
  const section = await addProject();
  const cid = await newSpace(aside, 'New Space', 'idea chat');
  await exchange('I have an idea: a search bar for the shop that filters products as you type.');
  await exchange('Decision: search is client side only for now, because the catalog is small (under 200 items).');
  const quick = sql(`select id from turn where space_id=${cid} and checkpoint is not null order by id`).split('\n');
  const checkpoints = main.getByRole('region', { name: 'Checkpoints' });
  await checkpoints.getByText('2 checkpoints').waitFor({ timeout: 15000 });
  check(
    quick.length === 2,
    'a free Space in a git repo lists its two checkpoints before the attach',
    `turns with a checkpoint: ${quick.join(',')}; row: ${await checkpoints.innerText()}`,
  );
  await addFeature(section, 'Search');
  await section.getByRole('link', { name: 'idea chat' }).click();
  await page.getByRole('heading', { name: 'idea chat', level: 1 }).waitFor();
  await shot('before-attach');
  await page.getByRole('button', { name: 'Attach to work item' }).click();
  const dialog = page.getByRole('dialog', { name: 'Attach to a work item' });
  await pick('Work item', 'Search', dialog);
  await shot('attach-form');
  await dialog.getByRole('button', { name: 'Attach', exact: true }).click();
  await page.getByText(/This Space is now attached to work item "Search"/).first().waitFor({ timeout: 15000 });
  await shot('attached');
  check(
    (await page.getByRole('button', { name: 'Attach to work item' }).count()) === 0,
    'the Attach to work item button is gone once the Space has a work item',
    `space row (work_item_id|kind): ${sql(`select work_item_id,kind from space where id=${cid}`)}`,
  );
  await page.waitForTimeout(1000);
  const left = await checkpoints.allInnerTexts();
  check(
    !left.some((t) => t.includes('2 checkpoints') || quick.some((id) => t.includes(`before turn ${id} `))),
    'the free-Space checkpoints, taken in another tree, have no Roll back button after the attach',
    `row: ${JSON.stringify(left)}; GET rollbackable: ${JSON.stringify((await getJson(`/turns?space=${cid}&rollbackable=1`)).map((t) => t.id))}`,
  );
  const under = section
    .locator('li', { has: page.getByRole('link', { name: 'Search', exact: true }) })
    .getByRole('link', { name: 'idea chat' });
  await under.waitFor();
  check(
    (await section.getByRole('heading', { name: 'Quick sessions' }).count()) === 0,
    'sidebar moves the Space under the work item',
    `space row: ${sql(`select id,work_item_id from space where id=${cid}`)}`,
  );
  await exchange('Go on.');
  const lead = sql(`select count(*) from message where space_id=${cid} and author='lead'`);
  check(lead === '3', 'the Space still answers after the attach', `lead messages: ${lead}`);
}

async function cap() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'cap chat');
  await exchange('Say hello.');
  sql(`update Space set token_budget=1 where id=${cid}`, true);
  await send('Say hello again.');
  const banner = page.getByRole('alert');
  await banner.waitFor({ timeout: 15000 });
  const reason = await banner.innerText();
  await shot('paused');
  check(/token budget of 1 used/.test(reason), 'banner shows the reason', reason.replace(/\s+/g, ' '));
  check(await page.getByLabel('Message').isDisabled(), 'composer disabled while paused');
  await page.reload();
  await page.getByRole('alert').waitFor();
  check(true, 'banner survives a reload', (await page.getByRole('alert').innerText()).replace(/\s+/g, ' '));
  await page.getByRole('button', { name: 'Continue' }).click();
  await page.getByRole('alert').waitFor({ state: 'hidden' });
  check(await page.getByLabel('Message').isEnabled(), 'Continue clears the banner and enables the composer');
  await shot('continued');
  await exchange('Say hello a third time.');
  await shot('ran-after-continue');
  const state = sql(`select token_budget,paused from Space where id=${cid}`);
  const lead = sql(`select count(*) from message where space_id=${cid} and author='lead'`);
  check(lead === '2', 'next message ran after Continue', `space row (budget|paused): ${state}; lead messages: ${lead}`);
}

async function pages() {
  await page.goto(base + '/');
  await submitProject('bad path', '/nonexistent-reef-ui');
  const err = page.getByText(/not a directory/);
  await err.waitFor({ timeout: 10000 });
  await shot('bad-path');
  check(true, 'a bad project path shows the server error', await err.innerText());
  await page.reload();
  const section = await addProject();
  await addFeature(section, 'Login');
}

async function capture() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  await page.goto(base + '/capture');
  await pick('Capture project', name);
  const text = `ui capture ${Date.now().toString(36)}`;
  await page.getByLabel('Capture text').fill(text);
  await page.getByLabel('Capture text').press('Enter');
  await page.getByText('Captured').waitFor({ timeout: 15000 });
  await shot('captured');
  check((await page.getByLabel('Capture text').inputValue()) === '', 'the text box is empty after capture');
  const row = sql(`select title from work_item where project_id=${pid} and type='task' and title='${text}'`);
  check(row === text, 'the captured text is stored as a task work item', row);

  // A failing request shows its error: the picked project is deleted before Enter.
  const section2 = await addProject();
  const name2 = (await section2.getAttribute('aria-label')) ?? '';
  const pid2 = sql(`select id from project where name='${name2}'`);
  await page.goto(base + '/capture');
  await pick('Capture project', name2);
  sql(`delete from project where id=${pid2}`, true);
  await page.getByLabel('Capture text').fill('should fail');
  await page.getByLabel('Capture text').press('Enter');
  const err = page.getByText(/unknown project/);
  await err.waitFor({ timeout: 15000 });
  await shot('error');
  check(true, 'a capture whose request fails shows the error', await err.innerText());
}

async function firstRun() {
  await page.goto(base + '/');
  await page.getByLabel('Your name').waitFor();
  await page.getByLabel('Your name').fill('Tester');
  await page.getByRole('button', { name: 'Continue' }).click();
  const folder = page.getByLabel('Repo folder');
  await folder.waitFor();
  check((await page.getByRole('button', { name: 'Choose…' }).count()) === 0, 'plain browser has no folder picker');
  await shot('browser');

  await page.addInitScript(() => {
    window.reef = {
      chooseFolder: async () => '/chosen/repo',
      setTheme: () => {},
      onUpdateReady: () => () => {},
    };
  });
  await page.reload();
  await page.getByRole('button', { name: 'Choose…' }).click();
  check((await folder.inputValue()) === '/chosen/repo', 'desktop folder picker fills the repo path', await folder.inputValue());
  await shot('picker');
}

async function contrast(screen) {
  const rows = await page.evaluate(() => {
    const cv = document.createElement('canvas');
    cv.width = cv.height = 1;
    const cx = cv.getContext('2d', { willReadFrequently: true });
    const rgba = (css) => {
      cx.clearRect(0, 0, 1, 1);
      cx.fillStyle = '#000';
      cx.fillStyle = css;
      cx.fillRect(0, 0, 1, 1);
      const d = cx.getImageData(0, 0, 1, 1).data;
      return [d[0], d[1], d[2], d[3] / 255];
    };
    const mix = (under, [r, g, b, a]) => [under[0] * (1 - a) + r * a, under[1] * (1 - a) + g * a, under[2] * (1 - a) + b * a];
    const hex = (c) => '#' + c.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
    const lum = (c) => {
      const [r, g, b] = c.map((v) => {
        const s = v / 255;
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    const root = getComputedStyle(document.documentElement);
    const names = {};
    for (const n of ['main', 'sidebar', 'card', 'bubble', 'field', 'raised', 'hover', 'active', 'search', 'track', 'paused-bg', 'failed-bg', 'danger-bg', 'fg', 'soft', 'muted', 'faint', 'pill', 'pill-ink', 'working', 'needs-you', 'paused', 'paused-fg', 'paused-ink', 'done', 'failed', 'failed-fg', 'failed-soft', 'danger', 'lead']) {
      const [r, g, b, a] = rgba(root.getPropertyValue(`--color-${n}`).trim());
      if (a === 1) (names[hex([r, g, b])] ??= []).push(n);
    }
    const name = (h) => names[h]?.join('/') ?? h;
    const out = new Map();
    for (const el of document.body.querySelectorAll('*')) {
      if (el.closest('svg, [disabled], [aria-disabled="true"]')) continue;
      const text = [...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.textContent.trim()).join(' ').trim();
      const r = el.getBoundingClientRect();
      const cs = getComputedStyle(el);
      if (!text || r.width === 0 || r.height === 0 || cs.visibility !== 'visible') continue;
      const chain = [];
      for (let a = el; a; a = a.parentElement) chain.unshift(a);
      let bg = [255, 255, 255];
      for (const a of chain) bg = mix(bg, rgba(getComputedStyle(a).backgroundColor));
      const fg = mix(bg, rgba(cs.color));
      const ratio = (Math.max(lum(fg), lum(bg)) + 0.05) / (Math.min(lum(fg), lum(bg)) + 0.05);
      const large = parseFloat(cs.fontSize) >= 24 || (parseFloat(cs.fontSize) >= 18.66 && Number(cs.fontWeight) >= 700);
      const key = `${hex(fg)}|${hex(bg)}`;
      if (!out.has(key)) out.set(key, { fg: name(hex(fg)), bg: name(hex(bg)), ratio, min: large ? 3 : 4.5, text: text.slice(0, 28) });
    }
    return [...out.values()].sort((a, b) => a.ratio - b.ratio);
  });
  const theme = await page.evaluate(() => document.documentElement.dataset.theme ?? 'dark');
  console.log(`contrast ${screen} (${theme}): text on background, token names`);
  for (const r of rows) console.log(`  ${r.ratio.toFixed(2).padStart(5)} ${r.ratio >= r.min ? 'ok  ' : 'LOW '} ${r.fg} on ${r.bg} | ${r.text}`);
  const low = rows.filter((r) => r.ratio < r.min);
  check(low.length === 0, `${screen} text is at least 4.5:1 (3:1 for large) in ${theme}`, `${rows.length} colour pairs, lowest ${rows[0]?.ratio.toFixed(2)}${low.length ? `, below: ${low.map((r) => `${r.fg} on ${r.bg}`).join(', ')}` : ''}`);
}

async function theme() {
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/teams.md)');
  const bodyBg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  const cdp = await page.context().newCDPSession(page);
  const system = (value) => cdp.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value }] });

  const section = await addProject();
  await addFeature(section, 'Login');
  const fid = sql('select id from work_item order by id desc limit 1');
  const pid = sql(`select project_id from work_item where id=${fid}`);
  await fetch(`${base}/memory`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: 'decision', body: 'Sessions, not JWT', why: 'one server', scope: `work_item:${fid}` }) });
  await newTeamSpace('duo', 'theme room');
  await send('DESIGN a short plan for a login form.');
  await rootRow('architect').first().waitFor({ timeout: 240000 });
  await idle();
  const spaceUrl = page.url();

  const screens = async (mode) => {
    await page.goto(spaceUrl);
    await rootRow('architect').first().waitFor();
    await shot(`space-${mode}`);
    await contrast('space');
    await page.goto(base + '/inbox');
    await main.getByRole('link', { name: 'Open', exact: true }).first().waitFor();
    await shot(`inbox-${mode}`);
    await contrast('inbox');
    await page.goto(`${base}/features/${fid}`);
    await page.getByRole('heading', { name: 'Login', level: 1 }).waitFor();
    await page.waitForTimeout(500);
    await shot(`feature-${mode}`);
    await contrast('feature page');
  };

  await page.goto(base + '/settings?section=notifications');
  await page.getByRole('heading', { name: 'Settings', level: 1 }).waitFor();
  const notify = await main.getByRole('region', { name: 'Notifications' }).innerText();
  check(/Turn on notifications|On|Blocked/.test(notify), 'the Notifications section has the permission control', notify.replace(/\s+/g, ' '));
  await main.getByRole('navigation', { name: 'Settings sections' }).getByRole('button', { name: 'Appearance' }).click();
  const dark = await bodyBg();
  check((await page.getByRole('radio', { name: 'Dark' }).isChecked()) && dark === 'rgb(33, 33, 33)', 'Settings opens on Dark with the dark main colour', dark);
  await shot('settings-dark');
  await screens('dark');

  await page.goto(base + '/settings?section=appearance');
  await page.getByRole('radio', { name: 'Light' }).check();
  const light = await bodyBg();
  check(light === 'rgb(255, 255, 255)' && (await page.evaluate(() => document.documentElement.dataset.theme)) === 'light', 'picking Light restyles the open page at once', light);
  await shot('settings-light');
  await page.reload();
  const reloaded = await bodyBg();
  check(reloaded === 'rgb(255, 255, 255)' && (await page.getByRole('radio', { name: 'Light' }).isChecked()), 'Light survives a reload', `${reloaded}; localStorage ${await page.evaluate(() => localStorage.getItem('reef.theme'))}`);
  await screens('light');

  await page.goto(base + '/settings?section=appearance');
  await page.getByRole('radio', { name: 'Match macOS' }).check();
  await system('light');
  const followsLight = await bodyBg();
  await system('dark');
  const followsDark = await bodyBg();
  check(followsLight === 'rgb(255, 255, 255)' && followsDark === 'rgb(33, 33, 33)', 'Match macOS follows prefers-color-scheme, both ways, without a reload', `light ${followsLight}, dark ${followsDark}`);
  await system('light');
  await page.reload();
  check((await bodyBg()) === 'rgb(255, 255, 255)' && (await page.getByRole('radio', { name: 'Match macOS' }).isChecked()), 'Match macOS survives a reload and follows the system', await bodyBg());
  await shot('settings-system-light');

  await page.getByRole('radio', { name: 'Dark' }).check();
  await system('light');
  check((await bodyBg()) === 'rgb(33, 33, 33)', 'Dark stays dark while the system is light', await bodyBg());
}

async function phone() {
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/teams.md)');
  const section = await addProject();
  const itemId = await addFeature(section, 'Login');
  const featureId = page.url().split('/').pop();
  const { id: cid } = await newTeamSpace('duo', 'phone room');
  await send('DESIGN a short plan for a login form.');
  const end = Date.now() + 120000;
  while ((await getJson('/inbox')).length === 0 && Date.now() < end) await page.waitForTimeout(1000);

  await page.evaluate(() => {
    window.__marker = 'kept';
  });
  await page.setViewportSize({ width: 390, height: 844 });
  const tabs = page.getByRole('navigation', { name: 'Tabs' });
  const overflow = () =>
    page.evaluate(() => {
      const m = document.querySelector('main');
      return { page: document.documentElement.scrollWidth - innerWidth, main: m.scrollWidth - m.clientWidth };
    });
  const small = () =>
    page.evaluate(() =>
      [...document.querySelectorAll('nav[aria-label="Tabs"] a, main button, main select, main input:not([type=checkbox])')]
        .map((el) => [el, (getComputedStyle(el, '::after').position === 'absolute' ? el.parentElement : el).getBoundingClientRect()])
        .filter(([, r]) => r.width > 0 && r.height > 0 && r.height < 44)
        .map(([el, r]) => `${el.tagName.toLowerCase()} "${(el.getAttribute('aria-label') ?? el.textContent ?? '').trim().slice(0, 24)}" ${Math.round(r.height)}px`),
    );
  const visit = async (step, tab, ready) => {
    await tabs.getByRole('link', { name: tab }).click();
    await ready.waitFor();
    await shot(step);
    const o = await overflow();
    check(o.page <= 0 && o.main <= 0, `${step} has no horizontal scroll at 390 px`, `page ${o.page}, main ${o.main}`);
    const s = await small();
    check(s.length === 0, `${step} has 44 px tap targets`, s.join(', '));
  };

  await page.getByRole('link', { name: 'Back' }).click();
  await page.getByRole('heading', { name: 'Spaces', level: 1 }).waitFor();
  check(await tabs.isVisible(), 'the tab bar shows below 768 px');
  check(!(await page.locator('aside:not([aria-label])').isVisible()), 'the sidebar is hidden below 768 px');
  await visit('today', /^Today/, page.getByLabel('Quick capture'));
  await page.getByLabel('Quick capture').fill('phone check');
  await page.getByRole('button', { name: 'Capture', exact: true }).click();
  await page.getByRole('status').filter({ hasText: 'Captured' }).waitFor();
  await visit('spaces', /^Spaces/, page.getByRole('heading', { name: 'Spaces', level: 1 }));
  const row = main.getByRole('link', { name: /#phone room/ });
  check(await row.isVisible(), 'the Spaces tab lists the Space under its feature', await main.innerText().then((t) => t.replace(/\s+/g, ' ').slice(0, 120)));
  await visit('needs-you', /^Needs you/, main.getByRole('link', { name: 'Open', exact: true }).first());
  const badge = await tabs.getByLabel(/waiting$/).textContent();
  check(badge === '1', 'the Needs you tab shows a count badge of 1', badge);
  await visit('tasks', /^Tasks/, main.getByText('todo: phone check'));
  await main.getByRole('button', { name: 'Board' }).click();
  await main.getByRole('group', { name: 'Todo' }).waitFor();
  await shot('tasks-board');
  const ob = await overflow();
  check(ob.page <= 0 && ob.main <= 0, 'the task board has no horizontal scroll at 390 px', `page ${ob.page}, main ${ob.main}`);
  await visit('teams', /^Teams/, page.getByRole('heading', { name: 'Teams', level: 1 }));

  const noScroll = async (step) => {
    const o = await overflow();
    check(o.page <= 0 && o.main <= 0, `${step} has no horizontal scroll at 390 px`, `page ${o.page}, main ${o.main}`);
  };
  const box = (loc) => loc.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right) };
  });
  const tree = path.join(data, 'worktrees', sql(`select project_id from Space where id=${cid}`), itemId);
  writeFileSync(path.join(tree, 'auth.go'), ['package auth', '', 'func check(r *http.Request) error {', '\ttok := r.URL.Query().Get("token")', '\treturn verify(tok)', '}', ''].join('\n'));

  await tabs.getByRole('link', { name: /^Spaces/ }).click();
  await main.getByRole('link', { name: /#phone room/ }).click();
  await page.getByRole('heading', { name: 'phone room', level: 1 }).waitFor();
  await rootRow('architect').first().waitFor();
  check(!(await tabs.isVisible()), 'the tab bar is hidden in a Space');
  const back = main.getByRole('link', { name: 'Back' });
  const backBox = await box(back);
  check((await back.getAttribute('href')) === '/spaces' && backBox.bottom - backBox.top >= 44, 'the Space has a 44 px Back link to /spaces', JSON.stringify(backBox));
  await shot('space');
  await noScroll('space');
  const tiny = await small();
  check(tiny.length === 0, 'space has 44 px tap targets', tiny.join(', '));

  await idle();

  await main.getByRole('button', { name: /reply in thread|replies in thread|^Reply$/ }).first().click();
  const thread = page.getByRole('complementary', { name: 'Thread' });
  await thread.waitFor();
  const tb = await box(thread);
  check(tb.top === 0 && tb.left === 0 && tb.right === 390 && tb.bottom === 844, 'a thread opens as its own full screen', JSON.stringify(tb));
  await shot('thread');
  await noScroll('thread');
  await thread.getByRole('button', { name: 'Close thread' }).click();
  await thread.waitFor({ state: 'hidden' });

  await main.getByRole('button', { name: 'Team', exact: true }).click();
  const team = page.getByRole('complementary', { name: 'Team' });
  await team.waitFor();
  const sb = await box(team.locator('xpath=..'));
  check(sb.bottom === 844 && sb.top > 100 && sb.left === 0 && sb.right === 390, 'the team opens as a bottom sheet', JSON.stringify(sb));
  await shot('team-sheet');
  await noScroll('team sheet');
  await page.getByRole('button', { name: 'Close sheet' }).click({ position: { x: 195, y: 40 } });
  await team.waitFor({ state: 'hidden' });

  await main.getByRole('button', { name: 'Inspector', exact: true }).click();
  const inspector = page.getByRole('complementary', { name: 'Inspector' });
  await inspector.getByText('By role').waitFor();
  const ib = await box(inspector.locator('xpath=..'));
  check(ib.bottom === 844 && ib.top > 100 && ib.left === 0 && ib.right === 390, 'the inspector opens as a bottom sheet', JSON.stringify(ib));
  await shot('inspector-sheet');
  await noScroll('inspector sheet');
  await page.getByRole('button', { name: 'Close sheet' }).click({ position: { x: 195, y: 40 } });
  await inspector.waitFor({ state: 'hidden' });

  await main.getByRole('button', { name: /^Diff/ }).click();
  await main.getByRole('link', { name: 'Expand diff' }).click();
  const files = main.getByRole('navigation', { name: 'Files' });
  await files.getByText('auth.go').waitFor({ timeout: 15000 });
  check(!(await tabs.isVisible()), 'the tab bar is hidden on the review page');
  const fb = await box(files);
  const sideBySide = await files.evaluate((el) => getComputedStyle(el).flexDirection);
  check(sideBySide === 'row' && fb.right - fb.left === 390, 'the files show as chips above the diff, not a side list', `${sideBySide} ${JSON.stringify(fb)}`);
  await main.getByRole('region', { name: 'Diff' }).locator('[data-line="4"]').click();
  await main.getByLabel('Comment on auth.go:4').fill('Log the bad token first.');
  await main.getByRole('button', { name: 'Add comment' }).click();
  await shot('review');
  await noScroll('review');
  const sendButtons = main.getByRole('button', { name: /^Send to/ });
  const sendBox = await box(sendButtons.first());
  check((await sendButtons.count()) === 1 && (await sendButtons.first().isEnabled()) && sendBox.bottom <= 844 && sendBox.right - sendBox.left >= 340, 'review has one Send button, full width at the bottom', JSON.stringify(sendBox));

  await page.setViewportSize({ width: 1024, height: 800 });
  await aside.getByRole('link', { name: /^Today/ }).waitFor();
  check(!(await tabs.isVisible()), 'the tab bar goes away at 1024 px');
  check(await page.evaluate(() => window.__marker === 'kept'), 'widening keeps the same page, no reload');
  await shot('widened');
}

async function stop() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'stop chat');
  await send('Remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools.');
  await working();
  await openTurn();
  await turnPanel.locator('div.bg-bubble:not([role])').first().waitFor({ timeout: 60000 });
  await page.waitForTimeout(1000);
  await page.getByRole('button', { name: 'Stop' }).click();
  await main.getByRole('button', { name: /Failed: interrupted/ }).waitFor({ timeout: 30000 });
  await shot('stopped');
  check(true, 'Stop shows the turn failed', sql(`select id,status,error from turn where space_id=${cid}`));
  const partial = sql(`select substr(replace(body, char(10), ' '),1,40) from message where space_id=${cid} and author='lead'`);
  const shown = await main.getByText(partial.replace(/[*_`]/g, ''), { exact: false }).count();
  check(Boolean(partial) && shown === 1, 'the partial reply shows once, not as the message and as the live block', `partial: ${JSON.stringify(partial)}; shown ${shown} times`);
  await exchange('What was the secret word? Answer with the word only.');
  await shot('resumed');
  const answer = sql(`select body from message where space_id=${cid} and author='lead' order by id desc limit 1`);
  check(/pineapple/i.test(answer), 'the next message resumes the session', `answer: ${answer}`);
}

async function retry() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'retry chat');
  await send('Write a 1500-word story about lighthouses. Use no tools.');
  await working();
  await page.waitForTimeout(4000);
  await page.getByRole('button', { name: 'Stop' }).click();
  const row = page.getByRole('listitem').filter({ hasText: /lead's turn \d+ failed\s*interrupted/i });
  await row.waitFor({ timeout: 30000 });
  await shot('failed-row');
  await row.getByRole('button', { name: 'Retry' }).click();
  await working();
  await shot('retrying');
  await idle();
  await row.waitFor({ state: 'hidden' });
  await shot('retried');
  const turns = sql(`select id||' '||role||' '||status||' retried='||retried from turn where space_id=${cid} order by id`);
  check(/^\d+ lead failed retried=1\n\d+ lead done retried=0$/.test(turns), 'Retry runs the failed wake as a new turn that ends done', turns);
}

const git = (dir, ...args) =>
  execFileSync('git', ['-C', dir, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
function ensureRepo() {
  try {
    git(projectDir, 'rev-parse', 'HEAD');
  } catch {
    git(projectDir, 'init', '-q', '-b', 'main');
  }
  if (!git(projectDir, 'ls-files', 'README.md')) {
    writeFileSync(path.join(projectDir, 'README.md'), 'hello\n');
    git(projectDir, 'add', '.');
    git(projectDir, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'first');
  }
}

async function rollback() {
  ensureRepo();
  const section = await addProject();
  const itemId = await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'rollback chat');
  const tree = path.join(data, 'worktrees', sql(`select project_id from Space where id=${cid}`), itemId);
  const state = () => [git(tree, 'rev-parse', 'HEAD'), git(tree, 'status', '--porcelain'), git(tree, 'log', '--oneline'), git(tree, 'ls-files')].join('\n--\n');

  await exchange('Say hello.');
  writeFileSync(path.join(tree, 'mine.txt'), 'my notes\n');
  const before = state();
  await send('Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.');
  await working();
  await idle();
  git(tree, 'add', '-A');
  git(tree, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'by the turn');
  const turnId = sql(`select id from turn where space_id=${cid} order by id desc limit 1`);
  const list = main.getByRole('region', { name: 'Checkpoints' });
  await list.getByText('2 checkpoints').waitFor({ timeout: 15000 });
  await shot('checkpoints');
  check(
    git(tree, 'log', '--oneline').includes('by the turn') && (await list.innerText()).includes('2 checkpoints'),
    'the two turns fold into one checkpoint row and the turn left a commit and new.txt',
    `files: ${git(tree, 'ls-files')}; log:\n${git(tree, 'log', '--oneline')}`,
  );
  let asked = '';
  page.once('dialog', (d) => {
    asked = d.message();
    d.accept();
  });
  await list.getByRole('button', { name: 'Roll back…' }).click();
  await shot('checkpoint-menu');
  await page.getByRole('menuitem', { name: new RegExp(`^Before turn ${turnId} · lead`) }).click();
  await page.getByText(`Rolled back to before turn ${turnId}.`).waitFor({ timeout: 15000 });
  await shot('rolled-back');
  check(asked.includes(`Roll back to before turn ${turnId}?`), 'Roll back asks to confirm', asked);
  const after = state();
  check(after === before, 'HEAD, status, log and files are as before the turn', `before:\n${before}\nafter:\n${after}`);
  check(
    !existsSync(path.join(tree, 'new.txt')) && existsSync(path.join(tree, 'mine.txt')),
    'new.txt is gone and mine.txt is back',
    `ls: ${readdirSync(tree).filter((f) => f !== '.git').join(' ')}`,
  );
  check(
    sql(`select count(*) from message where space_id=${cid} and author='reef' and body='Rolled back to before turn ${turnId}.'`) === '1',
    'the Space has the note from reef',
  );
  await exchange('Did anything change in the files since your last answer? Answer in one sentence.');
  // The note is added to the prompt when it is sent, so it is in claude's session file, not in wake_text.
  const sid = sql(`select session_id from turn where space_id=${cid} order by id desc limit 1`);
  const projects = path.join(homedir(), '.claude', 'projects');
  const file = readdirSync(projects)
    .map((d) => path.join(projects, d, `${sid}.jsonl`))
    .find(existsSync);
  const note = `reef wrote: Rolled back to before turn ${turnId}.`;
  check(Boolean(file) && readFileSync(file, 'utf8').includes(note), 'the next turn was told of the rollback', `${note} in ${file}`);
}

async function diff() {
  ensureRepo();
  const section = await addProject();
  const itemId = await addFeature(section, 'Login');
  const featureId = page.url().split('/').pop();
  const cid = await newSpace(main, 'New Space', 'diff chat');

  await page.goto(`${base}/features/${featureId}/review`);
  await main.getByText('No changes yet').waitFor({ timeout: 15000 });
  await shot('empty');
  await fetch(`${base}/spaces/${cid}/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: 'Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.' }),
  });
  const files = main.getByRole('navigation', { name: 'Files' });
  await files.getByText('new.txt').waitFor({ timeout: 30000 });
  await shot('diff');
  check(
    (await files.innerText()).replace(/\s+/g, ' ') === 'A new.txt' &&
      (await main.locator('[data-path="new.txt"][data-line="1"]').innerText()).includes('hi'),
    'the review page lists new.txt as A with its line, open since before the turn and never reloaded',
    await main.innerText(),
  );
  const json = await getJson(`/work-items/${featureId}/diff`);
  check(
    json.length === 1 && json[0].path === 'new.txt' && json[0].status === 'added' && json[0].hunks[0].lines[0].text === 'hi',
    'GET /work-items/{id}/diff returns the file with its added line',
    JSON.stringify(json),
  );

  await page.goto(`${base}/features/${featureId}`);
  const meta = main.getByRole('link', { name: /^1 file/ });
  await meta.waitFor();
  await shot('summary');
  check(
    (await meta.innerText()).replace(/\s+/g, ' ').includes('1 file +1 −0') &&
      (await meta.getAttribute('href')) === `/features/${featureId}/review`,
    'the feature page header meta line sums the changes and links to the review page',
    await meta.innerText(),
  );
  await page.goto(`${base}/spaces/${cid}`);
  await page.getByRole('button', { name: /^Diff \+1/ }).click();
  await main.getByRole('heading', { name: 'Changes', level: 1 }).waitFor();
  await main.getByRole('navigation', { name: 'Files' }).getByText('new.txt').waitFor();
  await shot('panel');
  check(page.url().endsWith(`/spaces/${cid}`), "the Space header's Diff button opens a side panel in the space", page.url());
  const scope = main.getByRole('group', { name: 'Diff scope' });
  check(
    (await scope.getByRole('button', { name: 'Current changes' }).getAttribute('aria-pressed')) === 'true' &&
      (await main.getByText('uncommitted', { exact: true }).isVisible()),
    'the Space panel opens on Current changes',
    await scope.innerText(),
  );
  const tree = path.join(data, 'worktrees', sql(`select project_id from Space where id=${cid}`), itemId);
  git(tree, 'add', '-A');
  git(tree, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'old work');
  writeFileSync(path.join(tree, 'later.txt'), 'later\n');
  const panelFiles = main.getByRole('navigation', { name: 'Files' });
  await panelFiles.getByText('later.txt').waitFor({ timeout: 15000 });
  await shot('panel-current');
  check(
    (await panelFiles.innerText()).replace(/\s+/g, ' ') === 'A later.txt',
    'Current changes lists only the file written since the commit',
    await panelFiles.innerText(),
  );
  await scope.getByRole('button', { name: 'Full diff' }).click();
  await panelFiles.getByText('new.txt').waitFor({ timeout: 15000 });
  await shot('panel-full');
  check(
    (await panelFiles.innerText()).replace(/\s+/g, ' ') === 'A later.txt A new.txt' &&
      (await main.getByText('vs main').isVisible()),
    'Full diff lists the committed and the new file against main',
    await panelFiles.innerText(),
  );
  const current = await getJson(`/work-items/${featureId}/diff?scope=current`);
  check(current.length === 1 && current[0].path === 'later.txt', 'GET /work-items/{id}/diff?scope=current returns only the uncommitted file', JSON.stringify(current));
  await page.setViewportSize({ width: 1280, height: 800 });
  for (let i = 0; i < 32; i++) writeFileSync(path.join(tree, `many-${String(i).padStart(2, '0')}.txt`), 'x\n');
  writeFileSync(path.join(tree, 'long.txt'), Array.from({ length: 400 }, (_, i) => `line ${i + 1}\n`).join(''));
  await panelFiles.getByText('long.txt').click({ timeout: 15000 });
  const diffView = main.getByRole('region', { name: 'Diff' });
  const last = diffView.locator('[data-path="long.txt"][data-line="400"]');
  await last.waitFor({ state: 'attached' });
  const wheel = async (box) => {
    const b = await box.boundingBox();
    await page.mouse.move(b.x + b.width / 2, b.y + Math.min(b.height, 800 - b.y) / 2);
    await page.mouse.wheel(0, 20000);
    await page.waitForTimeout(300);
    return box.evaluate((e) => e.scrollTop);
  };
  const diffTop = await wheel(diffView);
  const lastBox = await last.boundingBox();
  const headerBox = await main.getByRole('heading', { name: 'Changes', level: 1 }).boundingBox();
  await shot('panel-scroll');
  check(
    diffTop > 0 && lastBox.y + lastBox.height <= 800 && headerBox.y >= 0,
    'at 1280x800 the wheel scrolls the panel diff to the last line of a 400-line file, header stays',
    `scrollTop=${diffTop} last=${JSON.stringify(lastBox)} header=${JSON.stringify(headerBox)}`,
  );
  const filesTop = await wheel(panelFiles);
  check(filesTop > 0, 'the wheel scrolls the panel file list of 35 files on its own', `scrollTop=${filesTop}`);

  mkdirSync(path.join(tree, 'web/src/modules/deep'), { recursive: true });
  mkdirSync(path.join(tree, 'docs'));
  for (const f of ['web/src/modules/deep/one.ts', 'web/src/modules/deep/two.ts', 'docs/x.md']) writeFileSync(path.join(tree, f), `${f}\n`);
  const pressed = (name) => panelFiles.getByRole('button', { name, exact: true }).getAttribute('aria-pressed');
  const rows = () => panelFiles.locator(':scope > button').evaluateAll((els) => els.map((el) => el.innerText.replace(/\s+/g, ' ').trim()));
  const folder = panelFiles.getByRole('button', { name: 'web/src/modules/deep', exact: true });
  const one = panelFiles.getByRole('button', { name: 'A one.ts', exact: true });
  await panelFiles.getByText('web/src/modules/deep/one.ts', { exact: true }).waitFor({ timeout: 15000 });
  check(
    (await pressed('List view')) === 'true' && (await panelFiles.getByText('web/src/modules/deep/one.ts', { exact: true }).isVisible()),
    'desktop: the file list opens as a flat list of full paths',
    (await rows()).slice(0, 4).join(', '),
  );
  await panelFiles.getByRole('button', { name: 'Tree view', exact: true }).click();
  await folder.waitFor();
  await panelFiles.evaluate((el) => (el.scrollTop = 0));
  check(
    (await folder.getAttribute('aria-expanded')) === 'true' &&
      (await rows()).slice(0, 6).join(', ') === 'docs, A x.md, web/src/modules/deep, A one.ts, A two.ts, A later.txt',
    'Tree view: folders first, alphabetical, the single-child chain merged into web/src/modules/deep, open, files by basename',
    (await rows()).slice(0, 6).join(', '),
  );
  await shot('tree-panel');
  await folder.click();
  const closed = (await folder.getAttribute('aria-expanded')) === 'false' && (await one.count()) === 0;
  await folder.click();
  check(closed && (await folder.getAttribute('aria-expanded')) === 'true' && (await one.isVisible()), 'clicking the folder row hides its files, clicking again shows them');
  await one.click();
  await diffView.getByText('web/src/modules/deep/one.ts', { exact: true }).waitFor();
  check((await one.getAttribute('aria-current')) === 'true', 'clicking one.ts in the tree opens its diff', await diffView.locator('[data-line="1"]').innerText());

  await page.goto(`${base}/spaces/${cid}`);
  await page.getByRole('button', { name: /^Diff \+/ }).click();
  await folder.waitFor({ timeout: 15000 });
  check((await pressed('Tree view')) === 'true', 'Tree view survives a reload of the Space');
  await panelFiles.getByRole('button', { name: 'A long.txt', exact: true }).click();
  await last.waitFor({ state: 'attached' });
  const treeTop = await wheel(diffView);
  check(treeTop > 0, 'in Tree view at 1280x800 the wheel still scrolls the panel diff', `scrollTop=${treeTop}`);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/spaces/${cid}`);
  await page.getByRole('button', { name: /^Diff \+/ }).click();
  await panelFiles.getByText('long.txt').waitFor({ timeout: 15000 });
  check(
    (await panelFiles.getByRole('group', { name: 'File list view' }).count()) === 0 &&
      (await panelFiles.getByRole('button', { name: 'A web/src/modules/deep/one.ts', exact: true }).count()) === 1,
    'phone: no list/tree toggle, the chips show full paths',
    (await rows()).slice(0, 4).join(', '),
  );
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${base}/features/${featureId}/review`);
  await folder.waitFor({ timeout: 15000 });
  check((await pressed('Tree view')) === 'true', 'the review page opens in Tree view too');
  await shot('tree-review');
  await page.goto(`${base}/spaces/${cid}`);
  await page.getByRole('button', { name: /^Diff \+/ }).click();
  await page.setViewportSize({ width: 1200, height: 1000 });
  await main.getByRole('button', { name: 'Close diff' }).click();
  await main.getByRole('heading', { name: 'Changes', level: 1 }).waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: /^Diff \+/ }).click();
  await main.getByRole('link', { name: 'Expand diff' }).click();
  await page.waitForURL(`**/features/${featureId}/review`);
  check(true, "the panel's expand button opens the full review page", page.url());
}

async function memory() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  await aside.getByRole('button', { name: 'Switch project' }).click();
  await page.getByRole('menuitem', { name: 'Open project' }).click();
  await page.waitForURL(`**/projects/${pid}`);
  const memoryCard = main.getByRole('region', { name: 'Project memory' });
  const newItem = memoryCard.getByRole('textbox', { name: 'New project memory' });
  await newItem.waitFor();
  await shot('empty');
  check((await memoryCard.getByRole('listitem').count()) === 0, 'a new project has no memory items yet');
  const itemRow = () => sql(`select status||'|'||kind||'|'||author||'|'||role||'|'||scope_kind||':'||scope_id||'|'||body from memory where scope_kind='project' and scope_id='${pid}'`);
  await pick('Kind', 'decision', memoryCard);
  await newItem.fill('Use Go, not Python.');
  await memoryCard.getByRole('button', { name: 'Add', exact: true }).click();
  await memoryCard.getByText('Use Go, not Python.').waitFor();
  await shot('saved');
  check(
    itemRow() === `accepted|decision|user|user|project:${pid}|Use Go, not Python.`,
    'Add stores an accepted project item with author user',
    itemRow(),
  );
  check((await newItem.inputValue()) === '', 'the add box clears after Add');
  await page.reload();
  await memoryCard.getByText('Use Go, not Python.').waitFor();
  check(true, 'the saved item survives a reload');
  await memoryCard.getByRole('button', { name: /^Retire decision/ }).click();
  await memoryCard.getByText('Use Go, not Python.').waitFor({ state: 'hidden' });
  const retired = sql(`select status||'|'||(retired_at is not null) from memory where scope_kind='project' and scope_id='${pid}'`);
  check(retired === 'retired|1', 'Retire sets retired_at and the item leaves the list', retired);

  const baseField = main.getByRole('textbox', { name: 'Base branch' });
  const baseCard = main.getByRole('region', { name: 'Base branch' });
  check((await baseField.getAttribute('placeholder')) === 'default (origin HEAD)', 'the Base branch field shows the default as its placeholder');
  await baseField.fill('a b');
  await baseCard.getByRole('button', { name: 'Save' }).click();
  await baseCard.getByText('not a branch name').waitFor();
  check(sql(`select base_branch from project where id=${pid}`) === '', 'a base branch that is not a branch name is refused and not stored');
  await baseField.fill('dev');
  await baseCard.getByRole('button', { name: 'Save' }).click();
  await baseCard.getByText('Saved').waitFor();
  check(sql(`select base_branch from project where id=${pid}`) === 'dev', 'saving the Base branch stores it', sql(`select id,base_branch from project where id=${pid}`));
  await page.reload();
  await baseField.waitFor();
  await page.waitForTimeout(500);
  await shot('base-branch');
  const baseValue = await baseField.inputValue();
  check(baseValue === 'dev', 'the saved base branch survives a reload', baseValue);

  const id = Date.now().toString(36);
  await main.getByRole('button', { name: 'New feature' }).click();
  const featureDialog = page.getByRole('dialog', { name: 'New feature' });
  await featureDialog.getByLabel('Feature title').fill(`Pp ${id}`);
  await featureDialog.getByRole('button', { name: 'Add', exact: true }).click();
  await main.getByRole('link', { name: `Pp ${id}` }).waitFor();
  await shot('feature-added');
  check(
    sql(`select count(*) from work_item where project_id=${pid} and title='Pp ${id}'`) === '1',
    'New feature on the project page stores the work item and lists it',
  );

  const session = `quick-${Date.now().toString(36)}`;
  await main.getByRole('button', { name: 'New quick session' }).click();
  const sessionDialog = page.getByRole('dialog', { name: 'New quick session' });
  await sessionDialog.getByLabel('Name', { exact: true }).fill(session);
  await sessionDialog.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('heading', { name: session, level: 1 }).waitFor();
  check(
    sql(`select count(*) from Space where project_id=${pid} and work_item_id is null and name='${session}'`) === '1',
    'New quick session on the project page stores a Space with no feature and opens it',
  );
  await page.goto(`${base}/projects/${pid}`);
  await main.getByRole('link', { name: session }).waitFor();
  await shot('quick-session-listed');

  const fid = sql(`select id from work_item where project_id=${pid} and name='Pp ${id}'`);
  const reefBin = process.env.REEF_UI_REEF;
  const reefOut = execFileSync(reefBin, ['memory', '--data', data, 'add', 'fact', 'Tokens live in Keychain.', '--scope', `work_item:${fid}`, '--as', 'developer'], { encoding: 'utf8' });
  console.log(reefOut.trim());
  await page.goto(`${base}/features/${fid}`);
  const featureMemory = main.getByRole('region', { name: 'Memory' });
  await featureMemory.getByText('Tokens live in Keychain.').waitFor();
  await page.waitForTimeout(500);
  await shot('feature-memory');
  check(true, 'an item saved with reef memory add shows on the feature page', await featureMemory.innerText());
  await featureMemory.getByRole('button', { name: /^Retire fact/ }).click();
  await featureMemory.getByText('Nothing remembered yet').waitFor();
  const featureRetired = sql(`select status||'|'||(retired_at is not null) from memory where scope_kind='work_item' and scope_id='${fid}'`);
  check(featureRetired === 'retired|1', 'Retire on the feature page sets retired_at and the item leaves the list', featureRetired);
}

async function inspector() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const { id: cid } = await newTeamSpace('full', 'inspector room');
  await send('Say ok in one word.');
  await workingAs('lead');
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.');
  await workingAs('developer');
  await idle();

  await main.getByRole('button', { name: 'Inspector' }).click();
  const panel = page.getByRole('complementary', { name: 'Inspector' });
  const devRow = panel.locator('li', { hasText: 'developer' });
  await devRow.waitFor({ timeout: 15000 });
  await shot('panel');

  const json = await getJson(`/spaces/${cid}/inspector`);
  const leadRole = json.roles.find((r) => r.role === 'lead');
  const devRole = json.roles.find((r) => r.role === 'developer');
  check(!!leadRole && !!devRole, 'the API lists a lead role and a developer role', JSON.stringify(json.roles));

  const turnsLabel = (n) => `${n} turn${n === 1 ? '' : 's'}`;
  const tokens = (u) => {
    const n = u.inputTokens + u.outputTokens;
    return n < 1000 ? String(n) : `${(n / 1000).toFixed(1)}k`;
  };
  const leadText = await panel.locator('li', { hasText: 'lead' }).innerText();
  const devText = await devRow.innerText();
  check(
    leadText.includes(turnsLabel(leadRole.turns)) && leadText.includes(tokens(leadRole.usage)),
    'the lead row shows turn count and tokens from the API',
    leadText,
  );
  check(
    devText.includes(turnsLabel(devRole.turns)) && devText.includes(tokens(devRole.usage)),
    'the developer row shows turn count and tokens from the API',
    devText,
  );

  const sumCost = json.roles.reduce((n, r) => n + (r.costUsd ?? 0), 0);
  const all = (await panel.innerText()).replace(/\s+/g, ' ');
  const total = (label, value) => all.includes(`${label} ${value}`);
  check(
    total('Cost', `$${sumCost.toFixed(4)}`) &&
      total('Tokens', tokens({ inputTokens: json.roles.reduce((n, r) => n + r.usage.inputTokens + r.usage.outputTokens, 0), outputTokens: 0 })) &&
      total('Turns', json.roles.reduce((n, r) => n + r.turns, 0)),
    'the totals are the sums of the API roles',
    all,
  );

  const devTurn = json.turns.find((t) => t.role === 'developer');
  const turnSummary = panel.locator('summary', { hasText: new RegExp(`Turn ${devTurn.id}(?!\\d)`) });
  await turnSummary.click();
  await turnSummary.locator('xpath=..').locator('summary', { hasText: 'Prompt' }).click();
  await shot('turn-open');
  const body = await panel.innerText();
  check(body.includes(devTurn.prompt), 'expanding the developer turn shows its prompt', body);
  check(
    devTurn.toolCalls.length > 0 && body.includes(devTurn.toolCalls[0].name) && body.includes(devTurn.toolCalls[0].result),
    'the developer turn shows a tool call with its input and result',
    body,
  );
}

async function subagent() {
  writeFileSync(path.join(projectDir, 'sub.txt'), 'sub-hello\n');
  // The Space runs in a worktree, which only has what is committed.
  const git = (...args) => execFileSync('git', ['-C', projectDir, '-c', 'user.name=reef', '-c', 'user.email=reef@example.com', ...args], { stdio: 'pipe' });
  git('init', '-q');
  git('add', 'sub.txt');
  git('commit', '-q', '--allow-empty', '-m', 'sub.txt');
  const section = await addProject();
  await addFeature(section, 'Subagent');
  const cid = await newSpace(main, 'New Space', 'subagent room');
  await send(
    'Use the Agent tool to start a subagent that reads the file sub.txt in the current directory with the Read tool. Then tell me its contents.',
  );
  await working();
  await openTurn();
  const group = turnPanel.locator('details', { has: page.locator('summary', { hasText: /^sub-agent/ }) });
  await group.locator('summary', { hasText: /^sub-agent/ }).waitFor({ timeout: 120000 });
  const agentRow = turnPanel.locator('details', { has: page.locator('summary', { hasText: /^Agent/ }) }).first();
  const label = await agentRow.locator('summary').first().innerText();
  check(/^Agent\s+\S+/.test(label) && !label.includes('{'), 'the Agent call shows as a subagent row labelled from its input', label);
  const child = group.first().locator('summary', { hasText: /Read/ });
  await child.waitFor({ timeout: 30000 });
  const open = await group.first().evaluate((el) => el.open);
  check(open, 'while the subagent runs its nested tool calls are shown under it', await group.first().innerText());
  await shot('turn-panel');
  await group.first().evaluate((el) => !el.open || new Promise((res) => el.addEventListener('toggle', res, { once: true })));
  check(!(await group.first().evaluate((el) => el.open)), 'when the Agent call is done its group collapses', await group.first().locator('summary').first().innerText());
  await shot('turn-panel-done');
  await idle();

  const json = await getJson(`/spaces/${cid}/inspector`);
  const calls = json.turns.flatMap((t) => t.toolCalls);
  const call = calls.find((c) => c.name === 'Agent');
  const nested = (call?.children ?? []).map((c) => c.name);
  check(nested.length > 0 && nested.includes('Read') && !calls.some((c) => c.name === 'Read'), 'the inspector API nests the subagent tool calls under the Agent call', JSON.stringify(call));
  const stored = sql(`select count(*) from tool_call where parent_id is not null`);
  check(Number(stored) > 0, 'the subagent tool calls are stored with the Agent call as parent', `tool_call rows with a parent: ${stored}`);

  await main.getByRole('button', { name: 'Inspector' }).click();
  const panel = page.getByRole('complementary', { name: 'Inspector' });
  const turnRow = panel.locator('details').filter({ hasText: 'Prompt' }).first();
  await turnRow.locator('summary').first().click();
  const iGroup = turnRow.locator('details', { has: page.locator('summary', { hasText: /^sub-agent/ }) });
  await iGroup.locator('summary').waitFor();
  check(!(await iGroup.evaluate((el) => el.open)), 'a finished turn in the inspector shows the subagent group collapsed', await iGroup.locator('summary').innerText());
  const input = JSON.parse(call.input);
  const want = [input.subagent_type, input.description].filter(Boolean).join(' · ');
  check((await panel.innerText()).includes(`Agent ${want}`), 'the inspector labels the Agent call from its input', want);
  await shot('inspector-collapsed');
  await iGroup.locator('summary').click();
  check(await iGroup.evaluate((el) => el.open), 'expanding the group shows the nested calls', await iGroup.innerText());
  await shot('inspector-expanded');
}

async function review() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) {
    throw new Error('apply the team full first (teams/full.json, see features/teams.md)');
  }
  ensureRepo();
  const section = await addProject();
  const itemId = await addFeature(section, 'Login');
  const featureId = page.url().split('/').pop();
  const { id: cid } = await newTeamSpace('full', 'review room');
  const tree = path.join(data, 'worktrees', sql(`select project_id from Space where id=${cid}`), itemId);
  await send('@developer Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.');
  await workingAs('developer', 60000);
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes.');
  await workingAs('developer', 60000);
  await idle();
  await send('@developer Use the Bash tool to delete README.md. Then reply done.');
  await workingAs('developer', 60000);
  await idle();
  const root = (await getJson(`/messages?space=${cid}`))
    .filter((m) => m.author === 'developer' && m.parentId === null)
    .at(-1);
  check(!!root, 'the developer posted a third top-level message, its latest thread root', root && `message ${root.id}`);

  writeFileSync(path.join(tree, 'blob.bin'), Buffer.from([0, 1, 2, 0]));
  await page.goto(`${base}/features/${featureId}/review`);
  const files = main.getByRole('navigation', { name: 'Files' });
  const diffView = main.getByRole('region', { name: 'Diff' });
  const sendButton = main.getByRole('button', { name: /^Send to/ });
  await files.getByText('README.md').waitFor({ timeout: 15000 });
  const rows = (await files.getByRole('button').allInnerTexts()).map((r) => r.replace(/\s+/g, ' ')).join(', ');
  check(rows === 'D README.md, A blob.bin, A d.txt, A new.txt', 'the files are listed with A, M or D', rows);
  check(
    (await files.getByText('README.md').evaluate((el) => getComputedStyle(el).textDecorationLine)) === 'line-through',
    'the deleted file is struck through',
  );
  await files.getByText('blob.bin').click();
  check((await diffView.innerText()).includes('No text changes.'), 'a file without hunks shows No text changes', await diffView.innerText());
  await files.getByText('README.md').click();
  await shot('diff');
  const comment = async (file, line, label, text) => {
    await files.getByText(file).click();
    await diffView.locator(`[data-path="${file}"][data-line="${line}"]`).click();
    await main.getByLabel(label).fill(text);
    await main.getByRole('button', { name: 'Add comment' }).click();
  };
  await comment('new.txt', 1, 'Comment on new.txt:1', 'nice start');
  await comment('d.txt', 1, 'Comment on d.txt:1', 'rename this');
  await comment('README.md', 1, 'Comment on README.md:1 (removed)', 'was this needed?');
  check(
    (await main.getByText('3 comments').count()) === 1 && (await files.getByText('README.md').locator('xpath=..').innerText()).trim().endsWith('1'),
    'three drafts are counted in the header and per file',
    await main.innerText(),
  );
  await shot('comments');
  const before = Number(sql(`select count(*) from turn where space_id=${cid} and role='developer'`));
  await sendButton.click();
  // The review message matches no recorded scenario, so the turn may error and end before
  // the window ever shows "is working": poll the turn row instead of workingAs/idle.
  let after = before;
  for (let i = 0; i < 60 && after === before; i++) {
    await page.waitForTimeout(500);
    after = Number(sql(`select count(*) from turn where space_id=${cid} and role='developer'`));
  }
  await idle();
  await shot('sent');

  const sent = (await getJson(`/messages?space=${cid}`)).at(-1);
  check(
    sent.author === 'user' &&
      sent.body.includes('new.txt:1') &&
      sent.body.includes('d.txt:1') &&
      sent.body.includes('README.md:1 (removed)'),
    'the three comments made one user message, the deleted line marked (removed)',
    JSON.stringify(sent),
  );
  check(sent.parentId === root.id, "the message's parent is the developer's latest thread root", `parentId ${sent.parentId}, root ${root.id}`);
  check(after > before, 'a developer turn started for the review message', `turns before ${before}, after ${after}`);
  check(
    (await main.getByText('0 comments').count()) === 1 && (await sendButton.isDisabled()),
    'comments clear after a successful send',
  );
}

async function inbox() {
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/teams.md)');
  const context = page.context();
  await context.grantPermissions(['notifications']);
  await page.addInitScript(() => {
    const Native = window.Notification;
    window.__notes = [];
    window.Notification = class extends Native {
      constructor(title, options) {
        super(title, options);
        window.__notes.push({ title, body: options?.body, note: this });
      }
    };
  });
  const notes = () => page.evaluate(() => window.__notes.map(({ title, body }) => ({ title, body })));
  // True when the element is fully on screen and marked with the ring class (it stays 2 s).
  const landedOn = async (id) => {
    await page.waitForFunction((id) => document.getElementById(id)?.classList.contains('ring-2'), id, { timeout: 5000 }).catch(() => {});
    return page.evaluate((id) => {
      const el = document.getElementById(id);
      const r = el?.getBoundingClientRect();
      return { ringed: !!el?.classList.contains('ring-2'), inView: !!r && r.top >= 0 && r.bottom <= window.innerHeight };
    }, id);
  };
  const waitWaiting = async (cid, since) => {
    const end = Date.now() + 120000;
    while (Date.now() < end) {
      const item = (await getJson('/inbox')).find((i) => i.kind === 'waiting' && String(i.spaceId) === String(cid));
      if (item && item.at !== since) return item;
      await page.waitForTimeout(1000);
    }
    throw new Error('no new waiting item');
  };
  const sendApi = (cid, text) =>
    fetch(`${base}/spaces/${cid}/send`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }) });
  const section = await addProject();
  await addFeature(section, 'Login');
  const { id: cid } = await newTeamSpace('duo', 'inbox room');
  const permission = await page.evaluate(() => Notification.permission);
  check(permission === 'granted', 'the window may show notifications', permission);

  await send('DESIGN a short plan for a login form.');
  await workingAs('lead', 60000);
  await aside.getByRole('link', { name: /^Needs you/ }).click();
  await page.getByRole('heading', { name: 'Needs you', level: 1 }).waitFor();
  check(page.url().endsWith('/inbox'), 'the Needs you link opens /inbox', page.url());
  const waiting = main.getByRole('listitem').filter({ hasText: /Waiting.*inbox room/ });
  await waiting.waitFor({ timeout: 240000 });
  await shot('waiting-row');
  const rowText = (await waiting.textContent()) ?? '';
  check(
    /#inbox room/.test(rowText) && (await waiting.getByRole('link', { name: 'Open' }).count()) === 1 && (await waiting.getByRole('button', { name: 'Dismiss' }).count()) === 1 && (await main.getByRole('button', { name: 'Approve' }).count()) === 0,
    'the lead handed off to the architect with no approval; the answer shows as a waiting item with Open and Dismiss',
    rowText,
  );
  check(
    sql(`select count(*) from approval where space_id=${cid}`) === '0' && sql(`select group_concat(role||' '||status) from (select role,status from turn where space_id=${cid} order by id)`) === 'lead done,architect done',
    'the turns are lead done, architect done and no approval row exists',
    sql(`select group_concat(role||' '||status) from (select role,status from turn where space_id=${cid} order by id)`),
  );
  const badge = await aside.getByLabel(/waiting$/).textContent();
  check(badge === '1', 'the Inbox link shows a count badge of 1', badge);
  await page.waitForTimeout(3000);
  const fired = await notes();
  check(fired.length === 1 && /^Waiting: inbox room$/.test(fired[0].title), 'one notification fired for the item', JSON.stringify(fired));

  await page.evaluate(() => window.__notes[0].note.onclick());
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  let landed = await landedOn('latest-message');
  check(page.url().endsWith(`/spaces/${cid}#latest-message`) && landed.inView && landed.ringed, 'clicking the notification opens the Space at the newest message, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await aside.getByRole('link', { name: /^Needs you/ }).click();
  await waiting.waitFor();
  const before = (await getJson('/inbox')).find((i) => i.kind === 'waiting').at;
  await waiting.getByRole('link', { name: 'Open' }).click();
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  landed = await landedOn('latest-message');
  check(page.url().endsWith(`/spaces/${cid}#latest-message`) && landed.inView && landed.ringed, 'Open on a waiting item opens the Space at the newest message, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await shot('waiting-opened-space');

  await sendApi(cid, 'Say ok in one word. Use no tools.');
  const visible = await waitWaiting(cid, before);
  await page.waitForTimeout(3000);
  const whileOpen = await notes();
  check(whileOpen.length === 1, 'a new item of the open, visible Space fires no notification', JSON.stringify({ item: visible.text, notes: whileOpen.map((n) => n.title) }));

  await page.evaluate(() => Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' }));
  await sendApi(cid, 'Say fine in one word. Use no tools.');
  await waitWaiting(cid, visible.at);
  await page.waitForTimeout(3000);
  const whileHidden = await notes();
  check(whileHidden.length === 2 && /^Waiting: inbox room$/.test(whileHidden[1].title), 'the same Space hidden from view fires one notification', JSON.stringify(whileHidden.map((n) => n.title)));

  await page.goto(base + '/inbox');
  await waiting.waitFor();
  await page.waitForTimeout(3000);
  const afterReload = await notes();
  check(afterReload.length === 0, 'items present at load fire no notification', JSON.stringify(afterReload));

  const dismissed = (await getJson('/inbox')).find((i) => i.kind === 'waiting' && String(i.spaceId) === String(cid));
  await shot('before-dismiss');
  await waiting.getByRole('button', { name: 'Dismiss' }).click();
  await waiting.waitFor({ state: 'hidden', timeout: 30000 });
  await shot('dismissed');
  check(
    !(await getJson('/inbox')).some((i) => i.kind === 'waiting' && String(i.spaceId) === String(cid)) &&
      sql(`select dismissed_message_id from Space where id=${cid}`) === String(dismissed.messageId),
    'Dismiss on a waiting item removes it and stores the message it was pressed on',
    sql(`select dismissed_message_id from Space where id=${cid}`),
  );
  await page.reload();
  await page.getByRole('heading', { name: 'Needs you', level: 1 }).waitFor();
  await page.waitForTimeout(1000);
  check((await waiting.count()) === 0, 'the dismissed item stays gone after a reload');
  await sendApi(cid, 'Say ok in one word. Use no tools.');
  await waitWaiting(cid, dismissed.at);
  await waiting.waitFor({ timeout: 30000 });
  await shot('back-after-new-message');
  check(true, 'a new agent message brings the dismissed item back');
}

async function wrap() {
  const section = await addProject();
  await addFeature(section, 'Login');
  const cid = await newSpace(main, 'New Space', 'wrap chat');
  await exchange('We are building email login. Decision: sessions, not JWT, because there is one server.');
  await page.getByRole('button', { name: 'Wrap up' }).click();
  await working();
  const panel = page.getByRole('region', { name: 'Proposal' });
  await panel.waitFor({ timeout: 240000 });
  await shot('proposal');
  check(
    await page.getByRole('button', { name: 'Wrap up' }).isDisabled(),
    'Wrap up is disabled while a wrap-up waits for approval',
  );
  await panel.getByRole('button', { name: 'Reject' }).click();
  await page.getByText('rejected', { exact: true }).waitFor();
  const fid = sql(`select work_item_id from Space where id=${cid}`);
  check(
    sql(`select status from proposal where space_id=${cid} order by id desc limit 1`) === 'rejected' &&
      sql(`select count(*) from brief where work_item_id=${fid}`) === '0',
    'Reject leaves the brief unchanged',
    `proposals ${sql(`select id,status from proposal where space_id=${cid}`)}`,
  );
  await send('/wrap');
  await working();
  await panel.waitFor({ timeout: 240000 });
  await panel.getByRole('button', { name: 'Approve' }).click();
  await page.getByText(/approved: \d+ items saved/).waitFor();
  await shot('approved');
  await section.getByRole('link', { name: 'Login', exact: true }).click();
  await page.getByText('Version 1 by').waitFor();
  check(sql(`select count(*) from brief where work_item_id=${fid}`) === '1', '/wrap then Approve makes brief version 1', `brief rows ${sql(`select id,author from brief where work_item_id=${fid}`)}`);

  await page.goto(`${base}/spaces/${cid}`);
  await main.getByText('Wrapped', { exact: true }).waitFor();
  await shot('wrapped');
  check(
    (await main.getByRole('button', { name: 'Reopen' }).count()) === 1 &&
      (await page.getByLabel('Message').count()) === 0 &&
      (await page.getByRole('button', { name: 'Wrap up' }).count()) === 0,
    'an approved Space shows Wrapped and Reopen, no composer, no Wrap up',
    `closed_at ${sql(`select closed_at from space where id=${cid}`)}`,
  );
  const queued = (await getJson('/inbox')).filter((i) => i.spaceId === Number(cid));
  await page.goto(`${base}/inbox`);
  await page.getByRole('heading', { name: 'Needs you' }).waitFor();
  await shot('wrapped-needs-you');
  check(queued.length === 0, 'Needs you has no item for the wrapped Space', JSON.stringify(queued));
  await page.goto(`${base}/spaces/${cid}`);
  await main.getByText('Wrapped', { exact: true }).waitFor();
  await aside.getByRole('button', { name: /^Wrapped \(1\)/ }).click();
  await section.getByRole('link', { name: /#wrap chat/ }).waitFor();
  await shot('wrapped-sidebar');
  check(true, 'the Space is listed under Wrapped (1) in the sidebar');

  await main.getByRole('button', { name: 'Reopen' }).click();
  await page.getByLabel('Message').waitFor();
  check(sql(`select closed_at is null from Space where id=${cid}`) === '1', 'Reopen brings the composer back and clears closed_at');

  await section.getByRole('link', { name: 'Login', exact: true }).click();
  await main.getByRole('button', { name: 'Close feature' }).click();
  await page.getByRole('dialog', { name: 'Close feature' }).waitFor();
  await shot('close-dialog');
  await page.getByRole('dialog', { name: 'Close feature' }).getByRole('button', { name: 'Close feature' }).click();
  await main.getByRole('button', { name: 'Reopen' }).waitFor();
  await aside.getByRole('button', { name: /^Closed \(1\)/ }).click();
  await section.getByRole('link', { name: 'Login', exact: true }).waitFor();
  await shot('closed-feature');
  check(
    sql(`select closed_at is not null from work_item where id=${fid}`) === '1' &&
      sql(`select closed_at is not null from Space where id=${cid}`) === '1',
    'Close feature closes the feature and its Space; it is listed under Closed (1)',
  );
}

async function teams() {
  const missing = [];
  for (const name of ['duo', 'full']) if ((await fetch(`${base}/teams/${name}`)).status !== 200) missing.push(name);
  if (missing.length) throw new Error(`apply the teams first: missing ${missing.join(', ')} (see features/teams.md)`);
  const section = await addProject();
  await addFeature(section, 'Login');

  const { id: cid, options } = await newTeamSpace('duo', 'duo room', 'team-picked');
  check(options.includes('duo') && options.includes('full') && options.includes('solo'), 'the New Space form lists the teams', options.join(', '));
  const space = await getJson(`/spaces/${cid}`);
  check(space.team === 'duo' && space.role === 'lead', 'the Space is created on team duo', JSON.stringify(space));

  await send('DESIGN a short plan for a login form.');
  const leadSeen = await workingAs('lead', 60000);
  await shot('lead-working');
  check(leadSeen, 'the live turn names the lead while it runs');
  const architectSeen = await workingAs('architect');
  await shot('architect-working');
  check(architectSeen, 'then the live turn names the architect');
  await rootRow('architect').first().waitFor({ timeout: 240000 });
  await idle();
  const root = (await getJson(`/messages?space=${cid}`)).find((m) => m.author === 'architect' && m.parentId === null);
  check(!!root, 'the architect posted a top-level message', root && `message ${root.id}: ${root.body}`);
  const replyButton = rootRow('architect').last().getByRole('button');
  check((await replyButton.textContent()) === 'Reply', 'the architect root shows a Reply button');
  await shot('architect-posted');

  await replyButton.click();
  const pane = page.getByRole('complementary', { name: 'Thread' });
  await pane.waitFor();
  await shot('thread-open');
  await pane.getByLabel('Reply').fill('Which field would you validate first? Answer in this thread.');
  await pane.getByLabel('Reply').press('Enter');
  const threadSeen = await workingAs('architect', 120000);
  await shot('thread-architect-working');
  check(threadSeen, 'a reply in the thread runs the architect, not the lead');
  await pane.locator('li').nth(2).waitFor({ timeout: 240000 });
  await idle();
  await shot('thread-answered');
  const authors = await pane.locator('li').evaluateAll((items) => items.map((li) => li.querySelector('span.capitalize')?.textContent ?? 'user'));
  check(authors.join(',') === 'architect,user,architect', 'the Thread pane shows the root, the user reply and the architect answer', authors.join(','));
  const inThread = (await getJson(`/messages?space=${cid}`)).filter((m) => m.parentId === root.id).map((m) => m.author);
  check(inThread.join(',') === 'user,architect', 'both replies are stored under the root', inThread.join(','));
  await pane.getByRole('button', { name: 'Close' }).click();
  const count = await rootRow('architect').last().getByRole('button').textContent();
  check(count === '2 replies in thread', 'the main list shows the root with a replies button', count);
  await shot('main-list');

  await section.getByRole('link', { name: 'Login', exact: true }).click();
  await page.getByRole('heading', { name: 'Login', level: 1 }).waitFor();
  const handoff =
    'Reply with exactly one line and nothing else: an at sign immediately followed by the word developer, then a space, then the words: reply with one word, use no tools.';
  const roles = (id) => sql(`select group_concat(role) from (select role from turn where space_id=${id} order by id)`);
  const { id: oid } = await newTeamSpace('full', 'open room');
  await send(handoff);
  check(await workingAs('developer'), 'the lead hands off to the developer and the developer runs');
  await idle();
  check(
    sql(`select count(*) from approval where space_id=${oid}`) === '0' && roles(oid) === 'lead,developer',
    'no approval is stored and the turns are lead, developer',
    `approvals: ${sql(`select count(*) from approval where space_id=${oid}`)}; turns: ${roles(oid)}`,
  );
  check((await main.getByRole('button', { name: 'Approve' }).count()) === 0, 'the Space shows no Approve button');
  await shot('handoff-ran');
}

// Dispatches a native HTML5 drag sequence (dragstart on `card`, dragover and drop on
// `target`) through the DOM, since the window uses the draggable attribute, not mouse
// gestures a synthetic drag can trigger headless.
async function errorText(pattern, timeout = 15000) {
  const loc = main.getByText(pattern);
  try {
    await loc.waitFor({ timeout });
    return (await loc.textContent()) ?? '';
  } catch {
    return null;
  }
}

async function workshop() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) {
    throw new Error('apply the team full first (teams/full.json, see features/teams.md)');
  }
  const mcpBefore = '[{"command":"true","env":{"API_KEY":"sk-verify-9911"}}]';
  sql(`update team_member set mcp_servers = '${mcpBefore}', tools = '["Read","Grep","Glob","Edit","Write","Bash"]' where team_id = (select id from team where name = 'full') and name = 'developer'; update teammate set skills = '["ghost-skill"]' where id = (select teammate_id from team_member where team_id = (select id from team where name = 'full') and name = 'developer')`, true);
  mkdirSync(path.join(projectDir, '.claude/skills/workshop-skill'), { recursive: true });
  writeFileSync(path.join(projectDir, '.claude/skills/workshop-skill/SKILL.md'), '---\nname: workshop-skill\ndescription: Picked in the workshop scenario.\n---\n');

  const section = await addProject();
  const projectId = sql('select max(id) from project');
  await page.evaluate((id) => localStorage.setItem('reef.sidebar.projectId', id), projectId);
  await page.goto(`${base}/teams`);
  await main.getByRole('button', { name: 'full', exact: true }).click();
  await main.getByRole('button', { name: /^developer/ }).click();
  const personaBox = main.getByLabel('developer persona');
  await personaBox.waitFor();
  const marker = `WORKSHOP-MARKER-${Date.now().toString(36)}`;
  const persona = `${await personaBox.inputValue()} Always say ${marker} once.`;
  await personaBox.fill(persona);
  await main.getByRole('button', { name: /^Change creature/ }).click();
  await main.getByRole('button', { name: 'Otter', exact: true }).click();
  await shot('persona-edited');
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  await shot('persona-saved');
  check(
    sql(`select teammate.persona from team_member m join teammate on teammate.id = m.teammate_id where m.team_id = (select id from team where name = 'full') and m.name = 'developer'`) === persona,
    "saving the developer's persona in the window updates its teammate row",
  );
  check(
    sql(`select teammate.face from team_member m join teammate on teammate.id = m.teammate_id where m.team_id = (select id from team where name = 'full') and m.name = 'developer'`) === 'otter',
    "picking a creature for the developer in the window saves it to its teammate row",
  );
  check(
    sql(`select m.mcp_servers from team_member m join teammate on teammate.id = m.teammate_id where m.team_id = (select id from team where name = 'full') and m.name = 'developer'`) === mcpBefore,
    "the developer's mcpServers, left out of the window, are unchanged",
  );

  // Two leads: the same error reef team apply gives.
  await main.getByRole('button', { name: /^architect/ }).click();
  await main.getByLabel('architect lead').check();
  await main.getByRole('button', { name: 'Save' }).click();
  const twoLeads = await errorText(/needs exactly one lead role, has 2/);
  await shot('two-leads-error');
  check(!!twoLeads, 'an invalid team (two leads) shows the reef team apply error', twoLeads ?? '(not shown)');
  await main.getByLabel('architect lead').uncheck();

  // Unrestricted Bash on a role without write: the same error reef team apply gives.
  const bashBox = main.getByLabel('architect tool Bash');
  check(await bashBox.isDisabled(), 'the Bash checkbox of a read-only role is disabled', await bashBox.locator('..').getAttribute('title'));
  await main.getByLabel('architect add tool').fill('Bash(*)');
  await main.getByLabel('architect add tool').press('Enter');
  await main.getByRole('button', { name: 'Save' }).click();
  const badTool = await errorText(/cannot have unrestricted Bash/);
  await shot('bad-tool-error');
  check(!!badTool, 'an invalid team (unrestricted Bash without write) shows the reef team apply error', badTool ?? '(not shown)');
  await main.getByRole('button', { name: 'Remove Bash(*)', exact: true }).click();
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();

  // Skills and allowed tools are picked in the window and save; removing an MCP server removes it.
  const row = (col) => sql(`select ${col} from team_member m join teammate on teammate.id = m.teammate_id where m.team_id = (select id from team where name = 'full') and m.name = 'developer'`);
  await main.getByRole('button', { name: /^developer/ }).click();
  const ghost = main.getByText('ghost-skill').locator('..');
  check((await ghost.innerText()).includes('missing'), 'a picked skill that is not on disk shows as missing', await ghost.innerText());
  await main.getByRole('button', { name: 'Remove ghost-skill', exact: true }).click();
  await main.getByRole('combobox', { name: 'developer add skill', exact: true }).click();
  const offered = await page.getByRole('listbox', { name: 'developer add skill', exact: true }).getByRole('option').allInnerTexts();
  check(offered.some((o) => o.startsWith('workshop-skill')), "the claude skill picker offers the project's skill", `${offered.length} offered`);
  await page.getByRole('listbox', { name: 'developer add skill', exact: true }).getByRole('option', { name: /^workshop-skill/ }).click();
  check(!(await main.getByText('workshop-skill').locator('..').innerText()).includes('missing'), 'a picked skill that is on disk is not missing');
  check((await main.getByLabel('developer tool Read').isChecked()) && !(await main.getByLabel('developer tool WebFetch').isChecked()), 'the tool checkboxes show the role\'s tools');
  check(await main.getByLabel('developer tool mcp__mcp1__*').count() === 1, 'the tool picker has one entry for the role\'s MCP server');
  await main.getByLabel('developer tool WebFetch').check();
  await main.getByLabel('developer add tool').fill('Bash(git log:*)');
  await main.getByLabel('developer add tool').press('Enter');
  await main.getByLabel('developer tool Write').uncheck();
  await shot('role-skills-tools-mcp');
  await pick('developer backend', 'Codex', main);
  check((await main.getByLabel('developer tool Read').count()) === 0, 'the tool picker is hidden on a Codex role');
  await main.getByRole('combobox', { name: 'developer add skill', exact: true }).click();
  const codexOffered = await page.getByRole('listbox', { name: 'developer add skill', exact: true }).getByRole('option').allInnerTexts();
  check(!codexOffered.some((o) => o.startsWith('workshop-skill')), 'a Codex role is not offered the Claude skills', `${codexOffered.length} offered`);
  await page.keyboard.press('Escape');
  await pick('developer backend', 'Claude Code', main);
  await pick('developer permission mode', 'acceptEdits', main);
  await main.getByRole('button', { name: 'Remove mcp1 · true' }).click();
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  check(row('teammate.skills') === '["workshop-skill"]', "saving the developer's skills updates its teammate row", row('teammate.skills'));
  check(row('m.tools').includes('Bash(git log:*)') && row('m.tools').includes('"WebFetch"') && !row('m.tools').includes('"Write"'), "picking and unpicking allowed tools saves them to the team member row", row('m.tools'));
  check(row('m.mcp_servers') === '[]', "removing the developer's MCP server in the window removes it from its team member row", row('m.mcp_servers'));

  // Duplicate makes a new team `reef team list` (the same store) shows.
  const copyName = `workshop-copy-${Date.now().toString(36)}`;
  await main.getByRole('button', { name: 'Duplicate' }).click();
  const dup = page.getByRole('dialog', { name: 'New team' });
  await dup.getByLabel('Name', { exact: true }).fill(copyName);
  await dup.getByRole('button', { name: 'Create team' }).click();
  await main.getByRole('button', { name: copyName, exact: true }).waitFor({ timeout: 15000 });
  await shot('copied');
  check(
    sql(`select count(*) from team where name = '${copyName}'`) === '1',
    'Duplicate makes a new team',
  );

  // The next developer wake uses the new persona.
  await addFeature(section, 'Login');
  const { id: cid } = await newTeamSpace('full', 'workshop room');
  await send('@developer Say ok in one word.');
  await workingAs('developer');
  await idle();
  await shot('space-otter');
  const otters = await main.locator('svg [fill="#C08A5B"]').count();
  const penguins = await main.locator('svg [fill="#3FBFA0"]').count();
  check(otters > 0 && penguins === 0, "the Space shows the otter picked for the developer, not the penguin", `otter shapes ${otters}, penguin shapes ${penguins}`);
  const prompt = sql(`select prompt from turn where space_id = ${cid} and role = 'developer' order by id limit 1`);
  check(prompt.includes(marker), "the next developer wake's prompt carries the new persona", prompt.slice(0, 200));

  await page.goto(`${base}/settings?section=skills`);
  const skills = main.getByRole('region', { name: 'Skills' });
  await skills.getByRole('group', { name: 'Claude Code' }).waitFor();
  const claudeGroup = await skills.getByRole('group', { name: 'Claude Code' }).innerText();
  await shot('settings-skills');
  await skills.getByRole('button', { name: 'Codex' }).click();
  const codexGroup = await skills.getByRole('group', { name: 'Codex' }).innerText();
  check(claudeGroup.includes('workshop-skill') && claudeGroup.includes('Picked in the workshop scenario.'), 'Settings lists the project\'s Claude skill with its description', claudeGroup.replace(/\s+/g, ' ').slice(0, 160));
  check(!codexGroup.includes('workshop-skill'), 'Settings keeps the Claude skill out of the Codex group', codexGroup.replace(/\s+/g, ' ').slice(0, 160));
  await skills.getByRole('button', { name: 'Claude Code' }).click();
  await skills.getByLabel('Filter skills').fill('workshop');
  const filtered = await skills.getByRole('group', { name: 'Claude Code' }).getByRole('listitem').allInnerTexts();
  await shot('settings-skills-filter');
  check(filtered.length === 1 && filtered[0].includes('workshop-skill'), 'the skills filter keeps only the matching skill', filtered.join(' | '));
}

async function pr() {
  const gh = process.env.REEF_FAKE_GH;
  if (!gh) throw new Error('set REEF_FAKE_GH to the folder of the fake gh the server runs with');
  ensureRepo();
  const section = await addProject();
  const itemId = await addFeature(section, 'PR panel');
  const featureId = page.url().split('/').pop();
  const cid = await newSpace(main, 'New Space', 'pr chat');
  await fetch(`${base}/work-items/${featureId}/worktree`, { method: 'POST', headers: { 'Content-Type': 'application/json' } });

  const view = (over) => ({
    number: 12, title: 'Add the login page', url: 'https://github.com/x/y/pull/12', state: 'OPEN',
    isDraft: false, mergeable: 'CONFLICTING', reviewDecision: 'APPROVED', headRefName: `work-item/${itemId}`,
    baseRefName: 'main',
    statusCheckRollup: [
      { __typename: 'CheckRun', name: 'build', status: 'COMPLETED', conclusion: 'SUCCESS' },
      { __typename: 'CheckRun', name: 'lint', status: 'COMPLETED', conclusion: 'FAILURE' },
      { __typename: 'StatusContext', context: 'deploy preview', state: 'PENDING' },
    ],
    ...over,
  });
  const setPR = (over) => writeFileSync(path.join(gh, 'pr.json'), JSON.stringify(view(over)));
  setPR({});

  const chip = main.getByRole('button', { name: /^PR #12 / });
  await chip.waitFor({ timeout: 30000 });
  const panel = page.getByRole('complementary', { name: 'Pull request' });
  await chip.click();
  await panel.getByText('Add the login page').waitFor();
  await shot('conflicting');
  check(
    page.url().endsWith(`/spaces/${cid}`) && page.context().pages().length === 1,
    'the header PR chip opens the panel in the Space, not the browser',
    page.url(),
  );
  const merge = panel.getByRole('button', { name: 'Merge', exact: true });
  check(await merge.isDisabled(), 'Merge is disabled while GitHub says the PR is not mergeable', await panel.innerText());

  setPR({ mergeable: 'MERGEABLE' });
  await page.reload();
  await main.getByRole('button', { name: /^PR #12 / }).click();
  await panel.getByRole('button', { name: 'Merge', exact: true }).waitFor();
  await shot('panel');
  const text = (await panel.innerText()).replace(/\s+/g, ' ');
  const icons = await panel.getByRole('img').evaluateAll((els) => els.map((el) => el.getAttribute('aria-label')));
  check(
    text.includes('Add the login page') && text.includes('#12') && text.includes('Open') &&
      text.includes('main ← feature/') && text.includes('Approved') &&
      ['build', 'lint', 'deploy preview'].every((n) => text.includes(n)) && icons.join() === 'passed,failed,pending',
    'the panel shows title, number, state, base <- head, review and each check with its icon',
    `${text} | ${icons}`,
  );
  const link = panel.getByRole('link', { name: 'Open on GitHub' });
  check(
    (await link.getAttribute('href')) === 'https://github.com/x/y/pull/12' && (await link.getAttribute('target')) === '_blank',
    'Open on GitHub is an external link to the PR url',
  );
  check(await panel.getByRole('button', { name: 'Merge', exact: true }).isEnabled(), 'Merge is enabled for an open, mergeable PR');

  await panel.getByRole('button', { name: 'Merge', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Merge pull request' });
  await dialog.getByText('with a merge commit').waitFor();
  await shot('confirm');
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await dialog.waitFor({ state: 'hidden' });
  check(!existsSync(path.join(gh, 'merges.log')), 'Cancel in the dialog merges nothing');

  await panel.getByRole('button', { name: 'Merge', exact: true }).click();
  await dialog.getByRole('button', { name: 'Merge', exact: true }).click();
  await panel.getByText('Merged', { exact: true }).waitFor({ timeout: 15000 });
  await shot('merged');
  check(
    readFileSync(path.join(gh, 'merges.log'), 'utf8') === 'pr merge 12 --merge\n' &&
      (await panel.getByRole('button', { name: 'Merge', exact: true }).count()) === 0,
    'confirming runs gh pr merge 12 --merge once and the panel shows Merged with no Merge button',
    await panel.innerText(),
  );
  await main.getByRole('button', { name: 'PR #12 merged' }).waitFor({ timeout: 15000 });
  check(true, 'the header chip follows to merged', sql(`select body from message where space_id = ${cid} and author = 'reef' order by id desc limit 1`));
}

// Teammates' direct messages show as one compact item in the Space, never as bubbles. The
// lead's direct request runs the architect at once, and the fake architect answers slowly.
async function coordination() {
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/teams.md)');
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await addFeature(section, 'Login');
  const { id: cid } = await newTeamSpace('duo', 'coordination room');
  const panel = page.getByRole('complementary', { name: 'Coordination' });
  const items = page.getByRole('button', { name: /^lead and architect (are coordinating|coordinated) · \d+ messages?/i });
  const direct = () => getJson(`/spaces/${cid}/direct-messages`);
  const stateOf = async (item) => (await item.innerText()).split('\n').at(-1).trim();
  const until = async (item, state, timeout = 120000) => {
    const end = Date.now() + timeout;
    while (Date.now() < end) {
      if ((await item.count()) > 0 && (await stateOf(item)) === state) return true;
      await page.waitForTimeout(250);
    }
    return false;
  };
  const phoneWide = () => page.viewportSize().width < 768;
  let chains = 0;

  // One exchange: the item appears in `first` while the architect runs.
  async function exchange(label, text, first = 'working') {
    await send(text);
    chains += 1;
    const item = items.nth(chains - 1);
    check(await until(item, first), `${label}: one item says ${first}`, (await items.allInnerTexts()).join(' | '));
    check((await page.getByRole('button', { name: 'Approve' }).count()) === 0, `${label}: nothing waits for approval`);
    return item;
  }

  // Desktop: first exchange completes.
  const item1 = await exchange('desktop-1', 'COORDINATE the login review with the architect.');
  await item1.click();
  await panel.waitFor();
  // The open panel shows the running architect turn growing, one fake chunk a second.
  const liveText = async () => ((await panel.innerText()).match(/Reviewing the plan[^]*/) ?? [''])[0].replace(/\s+/g, ' ').trim();
  const grown = [];
  let inSpace = false;
  let midShot = false;
  const counts = [];
  const t0 = Date.now();
  for (const end = t0 + 15000; Date.now() < end && !grown.at(-1)?.includes('REVIEWED'); await page.waitForTimeout(200)) {
    const text = await liveText();
    counts.push(((await panel.innerText()).match(/Plan holds/g) ?? []).length);
    if (!midShot && Date.now() - t0 > 2500) {
      midShot = true;
      await shot('desktop-1-mid');
    }
    if (text && text !== grown.at(-1)) {
      grown.push(text);
      if (await main.locator('section').getByText('Reviewing the plan').count()) inSpace = true;
      console.log(`panel at ${Date.now() - t0} ms: ${text}`);
      if (grown.length <= 2) await shot(`desktop-1-live-${grown.length}`);
    }
  }
  check(
    grown.length >= 3 && grown.slice(1).every((t, i) => t.startsWith(grown[i])) && !grown[0].includes('Plan holds'),
    'the open Coordination panel shows the running direct turn text growing mid-turn',
    grown.join(' | '),
  );
  check(!inSpace, 'the Space narrative still shows no direct text while the panel grows');
  // From the live copy until the turn ends and the stored answer lands, the answer never shows twice.
  for (let settled = 0; settled < 8 && counts.length < 300; await page.waitForTimeout(200)) {
    counts.push(((await panel.innerText()).match(/Plan holds/g) ?? []).length);
    if ((await stateOf(item1)) === 'completed') settled += 1;
  }
  check(Math.max(...counts) === 1, 'the answer shows at most once while the live turn hands over to the stored one', counts.join(','));
  check(await until(item1, 'completed'), 'the answer completes the exchange');
  const stored1 = (await panel.innerText()).match(/Plan holds/g) ?? [];
  check(stored1.length === 1 && (await panel.getByRole('listitem').count()) === 2, 'after the turn the panel shows the stored response once, with no live copy', (await panel.innerText()).replace(/\s+/g, ' '));
  const rows = (await panel.getByRole('listitem').allInnerTexts()).map((t) => t.toLowerCase());
  check(
    rows[0].includes('lead → architect') && rows[0].includes('sent') && rows[0].includes('please review the login plan and tell me if it holds.'),
    'the panel lists the request with sender, recipient, time and delivery status',
    rows.join(' | '),
  );
  const done = await direct();
  const itemText = (await item1.innerText()).replace(/\s+/g, ' ').toLowerCase();
  check(
    done.length === 2 && itemText.includes('lead and architect coordinated · 2 messages') && (await items.count()) === 1,
    'the same item counts 2 messages instead of adding another',
    itemText,
  );
  await shot('desktop-1-completed');
  const userMessage = (await getJson(`/messages?space=${cid}`)).find((m) => m.author === 'user');
  const stored = done[0];
  check(stored.sourceMessageId === userMessage.id, 'the direct request stores the public message it was sent from', `sourceMessageId ${stored.sourceMessageId}, message ${userMessage.id}`);
  await panel.getByRole('button', { name: 'Open the message that started it' }).click();
  await page.getByRole('complementary', { name: 'Thread' }).getByText('COORDINATE the login review').waitFor();
  await shot('desktop-1-source');
  check(true, 'the source link opens the thread of the public message');
  await page.getByRole('button', { name: 'Close thread' }).click();
  await item1.click();
  await panel.getByRole('button', { name: 'Close coordination' }).click();
  check(
    (await page.getByText('Please review the login plan').count()) === 0 && (await page.getByText('Plan holds').count()) === 0,
    'no direct message renders as a bubble in the Space',
  );

  // Desktop: the second exchange is stopped and retried.
  const item2 = await exchange('desktop-2', 'COORDINATE the login review once more.');
  await page.getByRole('button', { name: 'Stop' }).click();
  check(await until(item2, 'stopped', 30000), 'Stop turns the item to stopped');
  await item2.click();
  await panel.waitFor();
  await shot('desktop-2-stopped');
  check((await panel.getByRole('button', { name: 'Retry' }).count()) === 1, 'the panel offers Retry for the stopped exchange');
  await panel.getByRole('button', { name: 'Retry' }).click();
  check(await until(item2, 'completed'), 'Retry runs the architect again and the exchange completes');
  const all = await direct();
  const secondChain = all.filter((m) => m.chainId === all[2].chainId);
  check(secondChain.map((m) => m.status).join(',') === 'sent,failed,done', 'the retried exchange keeps the failed attempt and the answer', secondChain.map((m) => m.status).join(','));
  await shot('desktop-2-retried');
  await panel.getByRole('button', { name: 'Close coordination' }).click();

  // Desktop: a recipient whose agent fails.
  const item3 = await exchange('desktop-3', 'COORDINATE the failing review with the architect.', 'failed');
  await item3.click();
  await panel.waitFor();
  await shot('desktop-3-failed');
  check((await panel.getByRole('button', { name: 'Retry' }).count()) === 1, 'the panel offers Retry for the failed exchange');
  await panel.getByRole('button', { name: 'Close coordination' }).click();

  // Phone: the same items and panel at 390x844.
  await page.setViewportSize({ width: 390, height: 844 });
  check((await items.count()) === 3, 'the phone width shows the same three items', (await items.allInnerTexts()).join(' | '));
  await shot('phone-items');
  const item4 = await exchange('phone', 'COORDINATE the login review on the phone.');
  await shot('phone-working');
  await page.getByRole('button', { name: 'Stop' }).click();
  check(await until(item4, 'stopped', 30000), 'the phone item says stopped');
  await item4.click();
  await panel.getByRole('button', { name: 'Retry' }).waitFor();
  await shot('phone-stopped');
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  check(wide <= 0 && phoneWide(), 'the panel has no horizontal scroll at 390 px', `page overflow ${wide}`);
  await panel.getByRole('button', { name: 'Close coordination' }).click();
  check(
    sql(`select count(*) from turn where space_id = ${cid} and status = 'failed' and error = 'interrupted'`) === '2',
    'both stopped exchanges stored an interrupted turn',
  );

  // The architect runs again, so the stopped turn can no longer be retried; it still reads stopped.
  await exchange('phone-2', 'COORDINATE the failing review once more.', 'failed');
  const kept = await stateOf(item4);
  check(kept === 'stopped', 'an earlier stopped item stays stopped after a later turn of its role', kept);
  await item4.click();
  await panel.waitFor();
  check((await panel.getByRole('button', { name: 'Retry' }).count()) === 0, 'it no longer offers Retry');
  await shot('phone-stopped-later');
  await panel.getByRole('button', { name: 'Close coordination' }).click();
}

async function team() {
  const section = await addProject();
  await addFeature(section, 'Login');
  await newTeamSpace('full', 'team room');
  await send('Say ok in one word.');
  await workingAs('lead');
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.');
  const running = await workingAs('developer');
  check(running, 'the developer turn is running');
  const avatars = main.getByRole('button', { name: 'Show team' });
  check((await avatars.locator('.c-ring').count()) === 1, 'the header avatars ring one role while it works');
  check((await page.getByText('working…', { exact: true }).count()) === 0, 'no floating working badge');
  const panel = page.getByRole('complementary', { name: 'Team' });
  check((await panel.count()) === 0, 'the team column is gone: nothing open until asked');
  const space = main.locator('section').first();
  const before = await space.boundingBox();
  await shot('closed-working');
  await main.getByRole('button', { name: 'Team', exact: true }).click();
  await panel.waitFor();
  const pop = await panel.locator('xpath=../..').boundingBox();
  const head = await main.locator('header').boundingBox();
  const after = await space.boundingBox();
  check(pop.width === 320 && pop.y >= head.y + head.height && pop.x + pop.width <= 1200, 'the popover is 320 px wide under the header, inside the window', JSON.stringify(pop));
  check(after.width === before.width, 'opening it does not change the Space width', `${before.width} -> ${after.width}`);
  const rows = await panel.getByRole('listitem').allInnerTexts();
  check(rows.length === 4 && rows.some((r) => /developer[\s\S]*working/i.test(r)), 'it lists four roles and the developer as working', rows.join(' | ').replace(/\n/g, ' '));
  await shot('popover-working');
  await page.keyboard.press('Escape');
  await panel.waitFor({ state: 'hidden' });
  await avatars.click();
  await panel.waitFor();
  await page.mouse.click(300, 600);
  await panel.waitFor({ state: 'hidden' });
  check(true, 'Escape and a click outside both close the popover');
  await main.getByRole('button', { name: 'Team', exact: true }).click();
  await panel.getByRole('button', { name: 'Close team' }).click();
  await panel.waitFor({ state: 'hidden' });
  check(true, 'its X closes it');
  await idle();
  check((await avatars.locator('.c-ring').count()) === 0, 'the ring goes when the turn ends');
  await shot('idle');
}

async function denials() {
  const roles = [
    { name: 'lead', lead: true, backend: 'claude', tools: ['Read'], persona: 'You are the lead. Do what the user asks with the tools you have, then answer in one short sentence.' },
    { name: 'reviewer', backend: 'claude', tools: ['Read'], persona: 'You are the reviewer. Do what the user asks with the tools you have, then answer in one short sentence. Never @mention anyone.' },
  ];
  const putTeam = (r) =>
    fetch(`${base}/teams/denials`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: 'denials', roles: r }) });
  check((await putTeam(roles)).status === 200, 'the team denials is applied: a lead and a read-only reviewer, both allowed only Read');
  const toolsOf = async (role) => (await getJson('/teams/denials')).roles.find((r) => r.name === role).tools.join(',');
  const turnLines = (role) =>
    readFileSync(process.env.REEF_FAKE_LOG ?? '/dev/null', 'utf8').split('\n').filter((l) => l.includes(` role=${role} `));
  const allowed = (line) => /--allowedTools (\S+)/.exec(line)?.[1];
  const pidOf = (line) => /pid=(\d+)/.exec(line)?.[1];

  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await addFeature(section, 'Fetch');
  const { id: cid } = await newTeamSpace('denials', 'denials room');
  const card = main.getByRole('listitem').filter({ hasText: / was denied [A-Z]/ });
  const add = card.getByRole('button', { name: 'Add to role' });
  const ask = async (text) => {
    await send(text);
    await working();
    await idle();
  };
  const fetchPrompt = (who, again) => `${who}Use the WebFetch tool ${again ? 'again ' : ''}to fetch https://example.com and tell me its title.`;

  await ask(fetchPrompt('@reviewer ', false));
  await card.waitFor({ timeout: 15000 });
  check((await card.innerText()).replace(/\s+/g, ' ').includes('Reviewer was denied WebFetch') && (await add.count()) === 1, "a denied tool call shows 'Reviewer was denied WebFetch' with Add to role", (await card.innerText()).replace(/\s+/g, ' '));
  await shot('reviewer-denied');
  check(await toolsOf('reviewer') === 'Read', 'the reviewer row still allows only Read', await toolsOf('reviewer'));
  await add.click();
  await card.getByText('Added', { exact: true }).waitFor();
  check((await add.count()) === 0 && (await toolsOf('reviewer')) === 'Read,WebFetch', 'Add to role stores WebFetch in the reviewer row and the card says Added', await toolsOf('reviewer'));
  await shot('reviewer-added');
  await page.reload();
  await card.getByText('Added', { exact: true }).waitFor();
  check((await add.count()) === 0, 'after a reload the card still says Added and offers no second Add');

  await ask(fetchPrompt('@reviewer ', true));
  const lines = turnLines('reviewer');
  const [before, after] = [lines.at(-2), lines.at(-1)];
  check(!allowed(before).includes('WebFetch') && allowed(after).includes('WebFetch'), 'the reviewer next start gets WebFetch in --allowedTools', `${allowed(before)} -> ${allowed(after)}`);
  const result = sql(`select result from tool_call where turn_id = (select max(id) from turn where space_id = ${cid} and role = 'reviewer') and name = 'WebFetch'`);
  check(result.startsWith('The page title is'), 'the next wake runs WebFetch successfully', result);
  check((await card.count()) === 0, 'the card is gone once the role has a newer turn');
  await shot('reviewer-retried');

  await ask('@reviewer Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened.');
  await card.waitFor({ timeout: 15000 });
  const text = (await card.innerText()).replace(/\s+/g, ' ');
  check(text.includes('Reviewer was denied Write') && text.includes('Needs write access') && (await add.count()) === 0, 'a read-only role denied Write shows Needs write access and no Add', text);
  await shot('write-denied');

  const flipped = roles.map((r) => (r.name === 'reviewer' ? { ...r, backend: 'codex', tools: [] } : r));
  await putTeam(flipped);
  await page.reload();
  await card.getByText('Codex ignores allowed tools').waitFor();
  check((await add.count()) === 0, 'a codex role shows no Add');
  await page.waitForTimeout(500);
  await shot('codex-denied');
  await putTeam(roles);

  const leadCard = card.filter({ hasText: 'Lead was denied' });
  const leadAdd = leadCard.getByRole('button', { name: 'Add to role' });
  await ask(fetchPrompt('', false));
  await leadCard.waitFor({ timeout: 15000 });
  check((await leadCard.innerText()).replace(/\s+/g, ' ').includes('Lead was denied WebFetch') && (await leadAdd.count()) === 1, "a denied lead shows 'Lead was denied WebFetch' with Add to role");
  await shot('lead-denied');
  await leadAdd.click();
  await leadCard.getByText('Added', { exact: true }).waitFor();
  check((await toolsOf('lead')) === 'Read,WebFetch', 'Add to role stores WebFetch in the lead row', await toolsOf('lead'));

  await ask(fetchPrompt('', true));
  const lead = turnLines('lead');
  check(pidOf(lead.at(-2)) !== pidOf(lead.at(-1)) && allowed(lead.at(-1)).includes('WebFetch'), 'the lead agent restarted and got WebFetch in --allowedTools', `pid ${pidOf(lead.at(-2))} -> ${pidOf(lead.at(-1))}, ${allowed(lead.at(-1))}`);
  const leadResult = sql(`select result from tool_call where turn_id = (select max(id) from turn where space_id = ${cid} and role = 'lead') and name = 'WebFetch'`);
  check(leadResult.startsWith('The page title is'), 'the lead next wake runs WebFetch successfully', leadResult);
  check(sql(`select count(*) from turn where space_id = ${cid} and status = 'failed'`) === '0', 'no turn failed');
  await shot('lead-retried');
}

async function composer() {
  const section = await addProject();
  await addFeature(section, 'Compose');
  await newSpace(main, 'New Space', 'compose chat');
  const field = main.getByLabel('Message');
  const box = field.locator('xpath=ancestor::div[contains(@class,"bg-field")][1]');
  const geometry = () =>
    box.evaluate((el) => {
      const r = el.getBoundingClientRect();
      const radius = parseFloat(getComputedStyle(el).borderTopLeftRadius);
      return { height: Math.round(r.height), left: Math.round(r.left), right: Math.round(r.right), radius: Math.min(radius, r.height / 2) };
    });
  const lines = (n) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join('\n');
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  // A synthetic drag or paste of one file: headless Chrome cannot start a real drag from the desktop.
  const fire = (target, type, name) =>
    target.evaluate(
      (el, [type, name]) => {
        const data = new DataTransfer();
        data.items.add(new File(['x'], name, { type: 'image/png' }));
        const init = { bubbles: true, cancelable: true };
        el.dispatchEvent(type === 'paste' ? new ClipboardEvent('paste', { ...init, clipboardData: data }) : new DragEvent(type, { ...init, dataTransfer: data }));
      },
      [type, name],
    );
  const views = [['desktop', 1280, 800], ['phone', 390, 844]];

  for (const [label, width, height] of views) {
    await page.setViewportSize({ width, height });
    await page.goto(page.url());
    await field.waitFor();
    await field.fill('');
    await page.waitForTimeout(600);
    await shot(`${label}-empty`);
    const empty = await geometry();
    await field.fill('one line');
    const one = await geometry();
    await shot(`${label}-one-line`);
    await field.fill(lines(30));
    const many = await geometry();
    const scrolls = await field.evaluate((el) => el.scrollHeight > el.clientHeight);
    await shot(`${label}-many-lines`);
    check(empty.height === one.height, `${label}: the empty box is as high as one line, the placeholder does not wrap`, `${empty.height}px empty, ${one.height}px with one line`);
    check(many.radius === one.radius && many.radius < many.height / 2 - 20, `${label}: the corner radius is the same at 1 and 30 lines`, `${one.radius}px at ${one.height}px high, ${many.radius}px at ${many.height}px`);
    check(many.height <= 220 && many.height > one.height, `${label}: the box grows then stops at the cap`, `${one.height}px -> ${many.height}px`);
    check(scrolls, `${label}: the text scrolls inside the capped box`);
    check(many.left >= 0 && many.right <= width && (await noOverflow()), `${label}: the box stays inside the window, no horizontal scroll`, `${many.left}..${many.right} of ${width}`);

    await field.fill('look at');
    await fire(box, 'dragenter', 'shot.png');
    await page.waitForTimeout(200);
    const cls = (await box.getAttribute('class')) ?? '';
    check(cls.includes('border-dashed') && (await field.getAttribute('placeholder')) === 'Drop to add the file path', `${label}: dragging a file over the box shows the drop state`);
    await fire(box, 'dragover', 'shot.png');
    await shot(`${label}-drag`);
    await fire(box, 'drop', 'shot.png');
    check((await field.inputValue()) === 'look at shot.png ' && !((await box.getAttribute('class')) ?? '').includes('border-dashed'), `${label}: a drop in a plain browser inserts the file name and ends the drop state`, JSON.stringify(await field.inputValue()));
    check((await main.getByText('No file path here: only the file name was added.').count()) === 1, `${label}: the browser says it has no path`);
    await shot(`${label}-dropped-name`);
    await field.fill('');
    await fire(field, 'dragenter', 'on-text.png');
    await fire(field, 'dragover', 'on-text.png');
    await page.waitForTimeout(200);
    check(((await box.getAttribute('class')) ?? '').includes('border-dashed'), `${label}: a drag over the text area itself shows the drop state`);
    await fire(field, 'drop', 'on-text.png');
    check((await field.inputValue()) === 'on-text.png ', `${label}: a drop on the text area itself inserts the file name`, JSON.stringify(await field.inputValue()));
    await field.fill('');
    await fire(field, 'paste', 'clip.png');
    check((await field.inputValue()) === 'clip.png ', `${label}: pasting a file in a plain browser inserts the file name`, JSON.stringify(await field.inputValue()));
    check((await page.getByRole('button', { name: /Paste file from clipboard/ }).count()) === 0, `${label}: a plain browser shows no clipboard chip`);
    await field.fill('');
  }

  // The desktop bridge is a stub: only the real Electron app has webUtils and the clipboard (see the desktop check by hand).
  await page.addInitScript(() => {
    window.__clip = null;
    const bridge = {
      pathForFile: (file) => `/Users/ada/Desktop/${file.name}`,
      clipboardFile: async () => window.__clip,
    };
    window.reef = new Proxy(bridge, {
      get: (target, key) => target[key] ?? (String(key).startsWith('on') ? () => () => {} : async () => null),
    });
  });
  for (const [label, width, height] of views) {
    await page.setViewportSize({ width, height });
    await page.goto(page.url());
    await field.waitFor();
    const chip = page.getByRole('button', { name: /Paste file from clipboard/ });
    check((await chip.count()) === 0, `${label}: no chip while the clipboard has no file`);
    await page.evaluate(() => {
      window.__clip = '/Users/ada/Desktop/my shot.png';
      window.dispatchEvent(new Event('focus'));
    });
    await chip.waitFor({ timeout: 5000 });
    await page.waitForTimeout(600);
    check((await chip.innerText()).includes('my shot.png'), `${label}: the window focus shows a chip named after the clipboard file`, (await chip.innerText()).replace(/\n/g, ' '));
    await shot(`${label}-chip`);
    await field.fill('see');
    await chip.click();
    check((await field.inputValue()) === 'see "/Users/ada/Desktop/my shot.png" ' && (await chip.count()) === 0, `${label}: the chip pastes the quoted path at the caret and goes away`, JSON.stringify(await field.inputValue()));
    await shot(`${label}-chip-pasted`);
    await field.fill('');
    await fire(box, 'dragenter', 'drop.png');
    await fire(box, 'drop', 'drop.png');
    check((await field.inputValue()) === '/Users/ada/Desktop/drop.png ', `${label}: a dropped file inserts its path`, JSON.stringify(await field.inputValue()));
    check((await main.getByText('No file path here').count()) === 0, `${label}: no name-only note when the path is known`);
    await field.fill('');
    await fire(field, 'paste', 'clip.png');
    check((await field.inputValue()) === '/Users/ada/Desktop/clip.png ', `${label}: a pasted file inserts its path`, JSON.stringify(await field.inputValue()));
    await field.fill('');
    await page.evaluate(() => {
      window.__clip = null;
      window.dispatchEvent(new Event('focus'));
    });
    await page.waitForTimeout(300);
    check((await chip.count()) === 0, `${label}: the chip is gone once the clipboard has no file`);
  }

  // Resizing the window with text in the box: it keeps its radius and stays inside the window.
  await page.setViewportSize({ width: 1280, height: 800 });
  await field.fill('a few words that wrap when the box is narrow, a few words that wrap when the box is narrow\nsecond line');
  for (const width of [1280, 1100, 900, 700, 390]) {
    await page.setViewportSize({ width, height: 800 });
    await page.waitForTimeout(300);
    const g = await geometry();
    check(g.radius === 28 && g.left >= 0 && g.right <= width && (await noOverflow()), `resize ${width}: radius 28, box inside the window`, `${g.left}..${g.right}, ${g.height}px high`);
    await shot(`resize-${width}`);
  }
  await field.fill('');
}

// A Space started with its name left empty gets a name from its provider after the first
// message; a typed name or a rename is never replaced.
async function naming() {
  await addProject();
  const probe = 'naming-probe: fix the login redirect loop. Answer in one or two sentences, use no tools.';
  const proposals = () =>
    (readFileSync(process.env.REEF_FAKE_LOG ?? '/dev/null', 'utf8').match(/scenario=name-login/g) ?? []).length;
  const startUnnamed = async () => {
    const was = page.url();
    await aside.getByRole('button', { name: 'New Space' }).click();
    await pick('Backend', 'claude', aside);
    await aside.getByRole('button', { name: 'Start' }).click();
    await page.waitForURL((u) => u.href !== was && /\/spaces\/\d+$/.test(u.pathname));
    return page.url().split('/').pop();
  };
  const row = (id) => sql(`select name, name_auto from Space where id=${id}`);
  const settled = async (id) => {
    for (let i = 0; i < 100 && sql(`select name_auto from Space where id=${id}`) !== '0'; i++) await page.waitForTimeout(200);
  };

  const first = await startUnnamed();
  const placeholder = row(first);
  check(/^claude \d.*\|1$/.test(placeholder), 'a Space started with an empty name has a placeholder name and waits for a proposal', placeholder);
  await shot('placeholder');
  await send(probe);
  await working();
  await idle();
  await aside.getByText('fix login redirect loop', { exact: true }).waitFor({ timeout: 20000 });
  await main.getByRole('heading', { name: /fix login redirect loop/, level: 1 }).waitFor();
  await shot('named');
  check(row(first) === 'fix login redirect loop|0', 'the provider name replaces the placeholder in the database, the sidebar and the Space header', row(first));
  const turns = sql(`select count(*) from turn where space_id=${first}`);
  const messages = sql(`select author from message where space_id=${first} order by id`).split('\n').join(',');
  check(turns === '1' && messages === 'user,lead', 'the naming call is not stored as a turn or a message', `turns: ${turns}, messages: ${messages}`);

  const second = await startUnnamed();
  await send('naming-fail: say hi. Answer in one or two sentences, use no tools.');
  await working();
  await idle();
  await settled(second);
  check(/^claude \d.*\|0$/.test(row(second)), 'a failed proposal keeps the placeholder name', row(second));
  await exchange('Say it again.');
  const done = sql(`select count(*) from turn where space_id=${second} and status='done'`);
  check(done === '2', 'the Space keeps working after a failed proposal', `done turns: ${done}`);
  await shot('failed-proposal');

  const before = proposals();
  const typed = await newSpace(aside, 'New Space', 'typed chat');
  await send(probe);
  await working();
  await idle();
  await page.waitForTimeout(2000);
  check(row(typed) === 'typed chat|0' && proposals() === before, 'a typed name is kept and no proposal is asked for', `${row(typed)}; proposals ${before} -> ${proposals()}`);

  const project = (await getJson('/projects')).at(-1).id;
  const post = (method, route, body) =>
    fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const created = await (await post('POST', '/spaces', { projectId: project, name: 'claude 02:32 PM', backend: 'claude' })).json();
  await post('POST', `/spaces/${created.id}/send`, { text: probe });
  const renamed = await post('PUT', `/spaces/${created.id}`, { name: 'mine' });
  for (let i = 0; i < 150 && sql(`select count(*) from message where space_id=${created.id} and author<>'user'`) === '0'; i++) await page.waitForTimeout(200);
  await page.waitForTimeout(2000);
  check(renamed.status === 200 && row(created.id) === 'mine|0', 'a rename sent right after the first message is never overwritten', `${row(created.id)}; PUT ${renamed.status}`);
}

async function hovercard() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) throw new Error('apply the team full first (see features/window.md)');
  ensureRepo();
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const itemId = await addFeature(section, 'Hover');
  const featureId = page.url().split('/').pop();
  const { id: cid } = await newTeamSpace('full', 'hover chat');
  await send('@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.');
  await working();
  await idle();

  const row = aside.getByRole('link', { name: /#hover chat/ });
  const tip = page.getByRole('tooltip');
  const away = () => page.mouse.move(900, 700);
  const flat = async () => (await tip.innerText()).replace(/\s+/g, ' ').toLowerCase();
  const roles = (await getJson('/teams/full')).roles.map((r) => r.name);
  const expected = async () => {
    let state;
    const files = await getJson(`/work-items/${featureId}/diff`);
    const lines = files.flatMap((f) => f.hunks.flatMap((h) => h.lines));
    const running = (await getJson('/turns?status=running')).filter((t) => String(t.spaceId) === cid).map((t) => t.role);
    const kinds = (await getJson('/inbox')).filter((i) => String(i.spaceId) === cid).map((i) => i.kind);
    state = kinds.includes('failed') ? 'a turn failed' : running.length ? 'working' : kinds.includes('waiting') ? 'waiting for you' : 'idle';
    const n = files.length;
    const changes = n ? `${n} file${n === 1 ? '' : 's'} +${lines.filter((l) => l.kind === 'add').length} −${lines.filter((l) => l.kind === 'del').length}` : 'no changes yet';
    return `#hover chat ${state} branch work-item/${featureId} changes ${changes} ${roles.map((r) => `${r} ${running.includes(r) ? 'working' : 'idle'}`).join(' ')}`;
  };

  await away();
  await row.hover();
  await page.waitForTimeout(150);
  check((await tip.count()) === 0, 'the card waits out its delay: nothing 150 ms after the pointer lands');
  await tip.waitFor({ timeout: 3000 });
  await shot('waiting');
  const idleText = await flat();
  check(idleText === (await expected()), 'waiting Space: name, state, branch, changes and every role match REST and SQLite', idleText);
  check(idleText.startsWith('#hover chat waiting for you') && /1 file \+1 −0/.test(idleText), 'the finished developer turn left a waiting item, so the card says waiting for you; the diff has d.txt with its one added line', idleText);
  const [r, t] = [await row.boundingBox(), await tip.boundingBox()];
  check(t.x >= r.x + r.width, 'the card sits beside the row, not over it', `row right ${r.x + r.width}, card left ${t.x}`);
  const described = await row.getAttribute('aria-describedby');
  check(described !== null && described === (await tip.getAttribute('id')), 'the row is described by the card (role tooltip)', described);

  await tip.hover();
  await page.waitForTimeout(500);
  check(await tip.isVisible(), 'moving the pointer from the row onto the card keeps it open');
  await away();
  await tip.waitFor({ state: 'hidden', timeout: 2000 });
  check((await row.getAttribute('aria-describedby')) === null, 'the card closes when the pointer leaves, and the description goes');

  for (const i of (await getJson('/inbox')).filter((i) => String(i.spaceId) === cid && i.kind === 'waiting')) {
    await fetch(`${base}/spaces/${cid}/dismiss`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ messageId: i.messageId }) });
  }
  await row.getByRole('img', { name: 'needs you' }).waitFor({ state: 'hidden', timeout: 15000 });
  await row.hover();
  await tip.waitFor();
  await shot('idle');
  const calm = await flat();
  check(calm.startsWith('#hover chat idle branch') && calm === (await expected()), 'idle Space: once the waiting item is dismissed the card says idle and still matches REST', calm);
  await page.keyboard.press('Escape');
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  check(true, 'Escape closes the card');

  await away();
  await row.hover();
  await tip.waitFor();
  await row.click();
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  await page.waitForTimeout(700);
  check((await tip.count()) === 0, 'clicking the row closes the card and it does not come back under a still pointer');

  await away();
  await row.hover();
  await tip.waitFor();
  await page.goBack();
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  await page.goForward();
  await row.waitFor();
  await page.waitForTimeout(700);
  check((await tip.count()) === 0, 'navigating (back) closes the card');

  await away();
  const toRow = async () => {
    await aside.getByRole('searchbox', { name: 'Search' }).focus();
    for (let i = 0; i < 60; i++) {
      await page.keyboard.press('Tab');
      if (await row.evaluate((el) => el === document.activeElement)) return true;
    }
    return false;
  };
  check(await toRow(), 'Tab reaches the Space row');
  await tip.waitFor({ timeout: 1000 });
  check((await row.getAttribute('aria-describedby')) === (await tip.getAttribute('id')), 'keyboard focus on the row shows the card, described by it');
  await page.keyboard.press('Shift+Tab');
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  check(true, 'moving focus off the row closes the card');

  await fetch(`${base}/spaces/${cid}/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: 'Remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools.' }),
  });
  for (let i = 0; i < 100 && !(await getJson('/turns?status=running')).some((x) => String(x.spaceId) === cid); i++) await page.waitForTimeout(100);
  await row.getByRole('img', { name: /is working$/ }).waitFor({ timeout: 10000 });
  await away();
  await row.hover();
  await tip.waitFor({ timeout: 3000 });
  await page.waitForTimeout(300);
  const runningText = await flat();
  await shot('running');
  const stillRunning = (await getJson('/turns?status=running')).filter((x) => String(x.spaceId) === cid);
  check(stillRunning.length === 1 && runningText === (await expected()), 'running turn: state Working and the running role shown working, matching /turns?status=running', `${runningText}; running ${JSON.stringify(stillRunning.map((x) => x.role))}`);
  await away();
  await working();
  await idle();

  await page.evaluate(() => localStorage.setItem('reef.theme', 'light'));
  await page.reload();
  await row.waitFor();
  await row.hover();
  await tip.waitFor();
  await page.waitForTimeout(300);
  await shot('light');
  check((await page.evaluate(() => document.documentElement.dataset.theme)) === 'light', 'light theme is on for the card');
  const tipLow = await tip.evaluate((el) => {
    const bg = getComputedStyle(el).backgroundColor;
    return { bg, text: getComputedStyle(el).color };
  });
  check(tipLow.bg === 'rgb(244, 244, 245)', 'the card takes the light card colour', JSON.stringify(tipLow));
  await contrast('hovercard');
  await away();
  await page.evaluate(() => localStorage.setItem('reef.theme', 'dark'));
  await page.reload();
  await row.hover();
  await tip.waitFor();
  await contrast('hovercard');
  await shot('dark');
  await away();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/spaces`);
  const phoneRow = main.getByRole('link', { name: /#hover chat/ }).first();
  await phoneRow.waitFor();
  await phoneRow.hover();
  await page.waitForTimeout(900);
  check((await tip.count()) === 0, 'phone layout (390x844): hovering a Space shows no card');
  await page.mouse.move(5, 5);
  for (let i = 0; i < 60; i++) {
    await page.keyboard.press('Tab');
    if (await phoneRow.evaluate((el) => el === document.activeElement)) break;
  }
  await page.waitForTimeout(500);
  check((await phoneRow.evaluate((el) => el === document.activeElement)) && (await tip.count()) === 0, 'phone layout: keyboard focus on a Space shows no card');
  await shot('phone');
}

async function mention() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) throw new Error('apply the team full first (see features/window.md)');
  ensureRepo();
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await addFeature(section, 'Mention');
  const { id: cid } = await newTeamSpace('full', 'mention chat');
  await send('@lead and @developer, @nobody too: remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools. @tester ');
  await working();

  const tip = page.getByRole('tooltip');
  const at = (name) => main.locator('.mention', { hasText: name, hasNotText: /^$/ }).first();
  const away = () => page.mouse.move(640, 760);
  const flat = async () => (await tip.innerText()).replace(/\s+/g, ' ').trim();
  const team = await getJson('/teams/full');
  const expected = async (name) => {
    const role = team.roles.find((r) => r.name === name);
    const running = (await getJson('/turns?status=running')).filter((t) => String(t.spaceId) === cid).map((t) => t.role);
    const models = { sonnet: 'Sonnet', opus: 'Opus', haiku: 'Haiku' };
    const backend = role.backend === 'claude' ? 'Claude Code' : 'Codex';
    const model = role.model ? ((role.backend === 'claude' && models[role.model]) || role.model) : 'default model';
    const tools = role.backend === 'claude' ? ` Tools ${role.tools.length}` : '';
    return `${name} ${backend} · ${model} Edit ${role.persona.split('\n')[0]} State ${running.includes(name) ? 'Working' : 'Idle'} Access ${role.write ? 'Can write' : 'Read-only'}${tools}`;
  };
  const sameCard = (shown, want) => shown.toLowerCase() === want.toLowerCase();
  const lead = at('@lead');
  const developer = at('@developer');
  const nobody = at('@nobody');
  check((await team.roles.every((r) => r.name !== 'nobody')) && (await lead.count()) === 1 && (await nobody.count()) === 1, 'the message shows @lead, @developer, @nobody and @tester as mentions, and the team has no nobody');

  await page.waitForTimeout(3500);
  await away();
  await lead.hover();
  await page.waitForTimeout(150);
  check((await tip.count()) === 0, 'the card waits out its delay: nothing 150 ms after the pointer lands');
  await tip.waitFor({ timeout: 3000 });
  await shot('working-dark');
  const shownLead = await flat();
  check(sameCard(shownLead, await expected('lead')) && shownLead.includes('State Working'), '@lead while the lead turn runs: role, backend and model, persona, Working, access and tools match /teams/full and /turns?status=running', shownLead);
  check(/Tools 3$/.test(shownLead) && /Read-only/.test(shownLead), 'the lead has 3 tools and is read-only', shownLead);
  const leadFace = await tip.locator('svg').first().innerHTML();
  const [m, t] = [await lead.boundingBox(), await tip.boundingBox()];
  check(t.x >= m.x + m.width, 'the card sits beside the mention, not over it', `mention right ${m.x + m.width}, card left ${t.x}`);
  const described = await lead.getAttribute('aria-describedby');
  check(described !== null && described === (await tip.getAttribute('id')), 'the mention is described by the card (role tooltip)', described);
  await tip.hover();
  await page.waitForTimeout(500);
  check(await tip.isVisible(), 'moving the pointer from the mention onto the card keeps it open');
  await away();
  await tip.waitFor({ state: 'hidden', timeout: 2000 });
  check((await lead.getAttribute('aria-describedby')) === null, 'the card closes when the pointer leaves, and the description goes');

  await developer.hover();
  await tip.waitFor({ timeout: 3000 });
  const shownDev = await flat();
  check(sameCard(shownDev, await expected('developer')) && shownDev.includes('State Idle') && shownDev.includes('Can write') && /Tools 6$/.test(shownDev), '@developer while only the lead runs: Idle, can write, 6 tools, matching /teams/full', shownDev);
  await shot('idle-developer');
  await page.keyboard.press('Escape');
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  check(true, 'Escape closes the card');
  await away();

  await nobody.hover();
  await page.waitForTimeout(900);
  check((await tip.count()) === 0 && (await nobody.getAttribute('tabindex')) === null, '@nobody (no such role in the team) shows no card and is not a tab stop');

  await lead.hover();
  await tip.waitFor();
  await lead.click();
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  await page.waitForTimeout(700);
  check((await tip.count()) === 0, 'clicking the mention closes the card and it does not come back under a still pointer');
  await away();

  const toMention = async (target) => {
    await page.getByLabel('Message').focus();
    for (let i = 0; i < 80; i++) {
      await page.keyboard.press('Shift+Tab');
      if (await target.evaluate((el) => el === document.activeElement)) return true;
    }
    return false;
  };
  check(await toMention(lead), 'Shift+Tab from the message box reaches the @lead mention');
  await tip.waitFor({ timeout: 1000 });
  check((await lead.getAttribute('aria-describedby')) === (await tip.getAttribute('id')), 'keyboard focus on the mention shows the card, described by it');
  await page.keyboard.press('Shift+Tab');
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  check(true, 'moving focus off the mention closes the card');

  await developer.hover();
  await tip.waitFor();
  await page.goBack();
  await tip.waitFor({ state: 'hidden', timeout: 1000 });
  await page.goForward();
  await lead.waitFor();
  await away();
  await page.waitForTimeout(700);
  check((await tip.count()) === 0, 'navigating (back) closes the card');

  await idle();
  await away();
  await lead.hover();
  await tip.waitFor({ timeout: 3000 });
  const calmLead = await flat();
  check(sameCard(calmLead, await expected('lead')) && calmLead.includes('State Idle') && (await getJson('/turns?status=running')).every((t) => String(t.spaceId) !== cid), '@lead once the turn is done: Idle, matching /turns?status=running', calmLead);
  await shot('idle-lead');
  await away();
  await tip.waitFor({ state: 'hidden', timeout: 2000 });

  await developer.hover();
  await tip.waitFor();
  const devFace = await tip.locator('svg').first().innerHTML();
  await tip.getByRole('link', { name: 'Edit' }).click();
  await page.waitForURL(/\/teams\?/);
  const url = new URL(page.url());
  check(url.pathname === '/teams' && url.searchParams.get('team') === 'full' && url.searchParams.get('role') === 'developer', 'Edit opens /teams with team=full and role=developer', page.url());
  const panel = page.getByRole('complementary', { name: 'Role' }).or(page.locator('[aria-label=Role]'));
  await panel.first().waitFor({ timeout: 5000 });
  const row = main.getByRole('button', { name: /^developer/ });
  check((await row.getAttribute('aria-pressed')) === 'true' && (await panel.first().getByLabel('developer name').inputValue()) === 'developer', 'the developer row is selected and its role panel is open');
  check((await main.getByRole('button', { name: /^lead/ }).getAttribute('aria-pressed')) === 'false', 'no other role is selected');
  check((await row.locator('svg').first().innerHTML()) === devFace && devFace !== leadFace, "the card's creature is the one the team page shows for developer, and differs from the lead's");
  await shot('edit');
  await page.goBack();
  await lead.waitFor();

  // The Thread pane is the right 400 px of the window: its mentions are the ones near the edge.
  const pane = page.getByRole('complementary', { name: 'Thread' });
  const edge = async () => {
    if ((await pane.count()) === 0) await main.getByRole('button', { name: /reply in thread|replies in thread|^Reply$/ }).first().click();
    await pane.waitFor();
    let best = null;
    for (const name of ['@lead', '@developer', '@tester']) {
      const el = pane.locator('.mention', { hasText: name }).first();
      const box = await el.boundingBox();
      if (!best || box.x + box.width > best.box.x + best.box.width) best = { el, name, box };
    }
    return best;
  };
  await away();
  const near = await edge();
  const tester = near.el;
  await tester.hover();
  await tip.waitFor({ timeout: 3000 });
  const [tm, tt] = [await tester.boundingBox(), await tip.boundingBox()];
  const width = 1280;
  check(tm.x + tm.width + 8 + tt.width > width, `the ${near.name} mention in the Thread pane is so near the right edge that a card on its right would run off the window`, `mention right ${tm.x + tm.width}, card ${tt.width} wide`);
  check(tt.x >= 0 && tt.x + tt.width <= width && tt.x + tt.width <= tm.x, 'the card flipped to the mention left and stays inside the window', `card ${tt.x}..${tt.x + tt.width}, mention left ${tm.x}`);
  const edgeRole = near.name.slice(1);
  check(sameCard(await flat(), await expected(edgeRole)), `${near.name} card matches /teams/full`, await flat());
  await shot('edge-dark');
  await away();

  await page.evaluate(() => localStorage.setItem('reef.theme', 'light'));
  await page.reload();
  const lightEdge = await edge();
  await lightEdge.el.hover();
  await tip.waitFor();
  await page.waitForTimeout(300);
  check((await page.evaluate(() => document.documentElement.dataset.theme)) === 'light', 'light theme is on for the card');
  await shot('edge-light');
  await contrast('mention card');
  await away();
  await tip.waitFor({ state: 'hidden', timeout: 2000 });
  await lead.hover();
  await tip.waitFor();
  await page.waitForTimeout(300);
  check((await flat()).startsWith('Lead '), 'the @lead card opens in light too');
  await shot('light');
  await away();
  await page.evaluate(() => localStorage.setItem('reef.theme', 'dark'));
  await page.reload();
  const darkEdge = await edge();
  await darkEdge.el.hover();
  await tip.waitFor();
  await page.waitForTimeout(300);
  await contrast('mention card');
  await away();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/spaces/${cid}`);
  const phoneLead = main.locator('.mention', { hasText: '@lead' }).first();
  await phoneLead.waitFor();
  await phoneLead.hover();
  await page.waitForTimeout(900);
  check((await tip.count()) === 0, 'phone layout (390x844): hovering a mention shows no card');
  await page.mouse.move(5, 5);
  await page.getByLabel('Message').focus();
  for (let i = 0; i < 80; i++) {
    await page.keyboard.press('Shift+Tab');
    if (await phoneLead.evaluate((el) => el === document.activeElement)) break;
  }
  await page.waitForTimeout(500);
  check((await phoneLead.evaluate((el) => el === document.activeElement)) && (await tip.count()) === 0, 'phone layout: keyboard focus on a mention shows no card');
  await shot('phone');
}

async function providers() {
  const fakeDir = process.env.REEF_UI_FAKE;
  if (!fakeDir || !existsSync(path.join(fakeDir, 'claude'))) throw new Error('set REEF_UI_FAKE to the folder with the fake claude (verify-reef ui does)');
  const name = `work-${Date.now().toString(36)}`;
  const teamName = `providers-${Date.now().toString(36)}`;
  const putTeam = (body) =>
    fetch(`${base}/teams/${teamName}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: teamName, ...body }) });
  const roleOf = async () => (await getJson(`/teams/${teamName}`)).roles.find((r) => r.name === 'lead');
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const setTheme = (t) => page.evaluate((t) => localStorage.setItem('reef.theme', t), t);
  const lead = { name: 'lead', lead: true, backend: 'claude', persona: 'You are the lead.' };
  check((await putTeam({ roles: [lead] })).status === 200, `the team ${teamName} is applied: one claude lead`);

  await page.setViewportSize({ width: 1280, height: 800 });
  await addProject();
  await page.goto(`${base}/settings?section=providers`);
  const pane = main.getByRole('region', { name: 'Providers' });
  const rows = pane.getByRole('listitem');
  await pane.getByRole('heading', { name: 'Providers' }).waitFor();
  await rows.first().waitFor();
  const detected = (await getJson('/providers')).filter((p) => p.detected);
  const text = (await rows.allInnerTexts()).map((t) => t.replace(/\s+/g, ' '));
  check(detected.length > 0 && detected.every((p) => text.some((t) => t.includes(p.name) && t.includes('Found on PATH') && t.includes(p.path))), 'the page lists every detected install with its path', text.join(' | '));
  check(detected.every((p) => !p.version || text.some((t) => t.includes(p.version))), 'a detected install shows its version when the server has one', JSON.stringify(detected.map((p) => p.version ?? null)));
  check(text.some((t) => t.includes('Default, Opus, Sonnet, Haiku')), 'a claude install lists the model names the role panel offers');
  check((await rows.filter({ hasText: 'Found on PATH' }).getByRole('button', { name: /^Remove/ }).count()) === 0, 'a detected install has no Remove');
  await shot('desktop-dark-list');

  const form = pane.getByRole('form', { name: 'Add provider' });
  const kind = (k) => form.getByRole('group', { name: 'Kind' }).getByRole('button', { name: k, exact: true }).click();
  const fill = async (k, folder, n) => {
    await kind(k);
    await form.getByLabel('Folder').fill(folder);
    await form.getByLabel('Name', { exact: true }).fill(n);
    await form.getByRole('button', { name: 'Add provider' }).click();
  };
  const bad = async (label, pattern, k, folder, n) => {
    await fill(k, folder, n);
    const msg = await errorText(pattern, 15000);
    check(!!msg, label, msg ?? '(not shown)');
  };
  check((await page.getByRole('button', { name: 'Choose…' }).count()) === 0, 'a plain browser has no Choose button, the folder is typed');
  await bad('a folder that does not exist shows the server error inline', /not a directory/, 'claude', '/nonexistent-reef-ui', name);
  await bad('a folder without the kind binary shows the server error inline', /no codex binary/, 'codex', fakeDir, name);
  await bad('a name taken by the one on PATH shows the server error inline', /taken by the one found on PATH/, 'claude', fakeDir, 'claude');
  await shot('desktop-dark-error');
  check((await getJson('/providers')).every((p) => p.detected), 'none of the bad adds stored a provider');

  await fill('claude', fakeDir, name);
  const added = rows.filter({ hasText: name });
  await added.waitFor();
  const stored = (await getJson('/providers')).find((p) => p.name === name);
  const addedText = (await added.innerText()).replace(/\s+/g, ' ');
  check(stored && !stored.detected && stored.folder === fakeDir && addedText.includes(fakeDir) && addedText.includes('Added') && (await added.getByRole('button', { name: `Remove ${name}` }).count()) === 1, 'adding the fake claude folder shows it as Added with its folder and a Remove', addedText);
  check((await form.getByLabel('Folder').inputValue()) === '' && (await form.getByLabel('Name', { exact: true }).inputValue()) === '', 'the form clears after a successful add');
  await shot('desktop-dark-added');
  await bad('the same name again shows the conflict inline', /already exists/, 'claude', fakeDir, name);

  await page.goto(`${base}/teams?team=${teamName}`);
  await main.getByRole('button', { name: /^lead/ }).click();
  const select = page.getByRole('combobox', { name: 'lead provider', exact: true });
  await select.waitFor();
  check((await select.innerText()).trim() === 'Default (PATH)', 'a role with no provider shows Default (PATH)');
  await select.click();
  const offered = await page.getByRole('listbox', { name: 'lead provider', exact: true }).getByRole('option').allInnerTexts();
  check(offered.join('|') === `Default (PATH)|${name}`, 'the claude role is offered PATH and the added claude install, not the detected one', offered.join('|'));
  await page.getByRole('listbox', { name: 'lead provider', exact: true }).getByRole('option', { name, exact: true }).click();
  await shot('desktop-dark-role');
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  check((await roleOf()).provider === name, 'saving the team stores the provider on the role (GET /teams)', JSON.stringify((await roleOf()).provider));
  await page.reload();
  await main.getByRole('button', { name: /^lead/ }).click();
  check((await page.getByRole('combobox', { name: 'lead provider', exact: true }).innerText()).trim() === name, 'after a reload the role shows its provider');

  await pick('lead backend', 'Codex', main);
  check((await page.getByRole('combobox', { name: 'lead provider', exact: true }).innerText()).trim() === 'Default (PATH)', 'changing the backend to Codex resets a claude provider');
  await pick('lead backend', 'Claude Code', main);
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  check(!(await roleOf()).provider, 'saving the reset role leaves no provider', JSON.stringify(await roleOf().then((r) => r.provider ?? null)));
  await pick('lead provider', name, main);
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();

  for (const [label, width, height] of [['desktop', 1280, 800], ['phone', 390, 844]]) {
    await page.setViewportSize({ width, height });
    for (const mode of ['dark', 'light']) {
      await setTheme(mode);
      await page.goto(`${base}/settings?section=providers`);
      await added.waitFor();
      await page.waitForTimeout(400);
      check(await noOverflow(), `${label} ${mode}: Settings Providers has no horizontal scroll`);
      await shot(`${label}-${mode}-page`);
      await page.goto(`${base}/teams?team=${teamName}`);
      await main.getByRole('button', { name: /^lead/ }).click();
      await page.getByRole('combobox', { name: 'lead provider', exact: true }).waitFor();
      await page.waitForTimeout(400);
      check(await noOverflow(), `${label} ${mode}: the role panel has no horizontal scroll`);
      await shot(`${label}-${mode}-role-panel`);
    }
  }
  await setTheme('dark');

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${base}/settings?section=providers`);
  await added.getByRole('button', { name: `Remove ${name}` }).click();
  await added.waitFor({ state: 'detached' });
  check((await getJson('/providers')).every((p) => p.name !== name), 'Remove deletes the added install', JSON.stringify((await getJson('/providers')).map((p) => p.name)));
  await shot('desktop-dark-removed');
  await page.goto(`${base}/teams?team=${teamName}`);
  await main.getByRole('button', { name: /^lead/ }).click();
  check((await page.getByRole('combobox', { name: 'lead provider', exact: true }).innerText()).trim() === name, 'a role keeps the name of a removed provider, as the server does');
}

try {
  await scenarios[scenario]();
} catch (err) {
  check(false, 'scenario aborted', String(err.message).split('\n')[0]);
  await shot('error').catch(() => {});
}
await browser.close();
process.exit(failed ? 1 : 0);

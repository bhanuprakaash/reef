// Drives the Reef window in Chrome and prints PASS/FAIL lines. See ../../SKILL.md.
// usage: node drive.mjs live|brief|promote|cap|pages|stop|wrap|teams|retry|rollback|inbox|diff|memory|inspector|review|board|workshop|capture BASE_URL
// env: REEF_UI_DATA (data folder of the server, for sqlite3), REEF_UI_PROJECT (project
// folder, default a new temp dir), REEF_UI_PROOF (screenshot folder, default ./proof),
// REEF_VERIFY_CHROME (path of a Chrome or Chromium binary; default the system Chrome).
import { chromium } from 'playwright-core';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import path from 'node:path';

const [scenario, base] = process.argv.slice(2);
const scenarios = { live, brief, promote, cap, pages, stop, wrap, teams, retry, rollback, inbox, diff, memory, inspector, review, board, workshop, capture };
if (!scenarios[scenario] || !base) {
  console.error('usage: node drive.mjs live|brief|promote|cap|pages|stop|wrap|teams|retry|rollback|inbox|diff|memory|inspector|review|board|workshop|capture BASE_URL');
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

async function send(text) {
  await page.getByLabel('Message').fill(text);
  await page.getByLabel('Message').press('Enter');
}
async function working() {
  await page.getByText('is working...').waitFor({ timeout: 30000 });
}
async function idle() {
  await page.getByText('is working...').waitFor({ state: 'hidden', timeout: 240000 });
  await page.getByRole('button', { name: 'Stop' }).waitFor({ state: 'hidden' });
}
async function exchange(text) {
  await send(text + ' Answer in one or two sentences, use no tools.');
  await working();
  await idle();
}

// Fills the add-project form: the first-run screen when the server has no project, else the project switcher's.
async function submitProject(name, folder) {
  const switcher = aside.getByRole('button', { name: 'Switch project' });
  await page.getByLabel('Repo folder').or(switcher).waitFor();
  if (await page.getByLabel('Repo folder').count()) {
    await page.getByLabel('Repo folder').fill(folder);
    await page.getByLabel('Name', { exact: true }).fill(name);
    await page.getByRole('button', { name: 'Add project' }).click();
    return;
  }
  await switcher.click();
  await page.getByRole('button', { name: 'Add project' }).click();
  await page.getByLabel('Project name').fill(name);
  await page.getByLabel('Project path').fill(folder);
  await page.getByRole('button', { name: 'Add', exact: true }).click();
}

// Adds a project through the form; returns its sidebar section.
async function addProject() {
  const name = `ui-${scenario}-${Date.now().toString(36)}`;
  // A channel of a feature runs in a git worktree of the project, so the folder needs a commit.
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
// Adds a feature and returns its slug. The slug gets a suffix: every scenario's project is the
// same git repo, and a feature's branch can be checked out in one worktree only.
async function addFeature(section, prefix, name) {
  const slug = `${prefix}-${Date.now().toString(36)}`;
  await section.getByRole('button', { name: 'Add feature' }).click();
  await section.getByLabel('Feature slug').fill(slug);
  await section.getByLabel('Feature name').fill(name);
  await section.getByRole('button', { name: 'Add', exact: true }).click();
  await section.getByRole('link', { name, exact: true }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return slug;
}
// Starts a claude channel from the form in `box` and returns its id.
async function newChannel(box, openName, name, mode = 'acceptEdits') {
  await box.getByRole('button', { name: openName }).click();
  await box.getByLabel('Backend', { exact: true }).selectOption('claude');
  await box.getByLabel('Permission mode', { exact: true }).selectOption(mode);
  await box.getByLabel('Channel name').fill(name);
  await box.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return sql(`select id from channel where name='${name}' order by id desc limit 1`);
}

// The label of the running turn names its role: `<role> is working...`.
const liveLabel = main.getByText('is working...').locator('..');
async function workingAs(role, timeout = 240000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if ((await liveLabel.count()) > 0) {
      const text = (await liveLabel.first().textContent().catch(() => '')) ?? '';
      if (text.replace('is working...', '').trim() === role) return true;
    }
    await page.waitForTimeout(250);
  }
  return false;
}
// The top-level message rows of the main list that `author` wrote (they have the Reply button).
const rootRow = (author) =>
  main.locator('ul > li:has(> button)').filter({ has: page.getByText(author, { exact: true }) });
// Starts a channel on `team` from the form (backend and permission mode stay the team's).
async function newTeamChannel(team, name, step) {
  await main.getByRole('button', { name: 'New channel' }).click();
  const picker = main.getByLabel('Team', { exact: true });
  await picker.locator('option', { hasText: team }).waitFor({ state: 'attached' });
  const options = await picker.locator('option').allTextContents();
  await picker.selectOption(team);
  await main.getByLabel('Channel name').fill(name);
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
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'live chat');
  await send(
    'List the files in this directory with a tool, then read one of them with a tool, then summarize what you saw in two sentences.',
  );
  await working();
  await main.locator('details summary').first().waitFor({ timeout: 120000 });
  await page.waitForTimeout(1500);
  await shot('midturn');
  const rows = await main.locator('details summary').count();
  check(rows >= 1, 'tool rows shown while the turn runs', `${rows} rows, first: ${await main.locator('details summary').first().innerText()}`);
  await idle();
  await shot('final');
  const lead = sql(`select count(*) from message where channel_id=${cid} and author='lead'`);
  check(lead === '1', 'reply stored as one lead message', `message rows:\n${sql(`select id,author,substr(body,1,100) from message where channel_id=${cid}`)}`);
  check((await main.locator('details').count()) === 0, 'live view cleared after the turn');
}

async function brief() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const featureId = page.url().split('/').pop();
  const res = await fetch(`${base}/features/${featureId}/brief`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ body: 'Goal: build the login page with magic links.', author: 'user' }),
  });
  check(res.status === 201, 'brief stored over REST', `PUT brief -> ${res.status}`);
  await page.reload();
  const cid = await newChannel(main, 'New channel', 'brief check', '');
  await send('what is the goal of this feature?');
  await working();
  await idle();
  await shot('answer');
  const answer = sql(`select body from message where author='lead' and channel_id=${cid}`);
  console.log(`answer: ${answer}`);
  check(/magic/i.test(answer), 'a new channel answers from the brief', 'answer mentions magic links');
}

async function promote() {
  ensureRepo();
  const section = await addProject();
  const cid = await newChannel(aside, 'New channel', 'idea chat');
  await exchange('I have an idea: a search bar for the shop that filters products as you type.');
  await exchange('Decision: search is client side only for now, because the catalog is small (under 200 items).');
  const quick = sql(`select id from turn where channel_id=${cid} and checkpoint is not null order by id`).split('\n');
  const rows = main.getByRole('region', { name: 'Checkpoints' }).getByRole('listitem');
  await rows.first().waitFor({ timeout: 15000 });
  check(
    quick.length === 2 && (await rows.count()) === 2,
    'a quick session in a git repo lists its two checkpoints before the promote',
    `turns with a checkpoint: ${quick.join(',')}; rows: ${await rows.count()}`,
  );
  await shot('before-promote');
  await page.getByRole('button', { name: 'Promote' }).click();
  await page.getByLabel('Feature slug').fill('search');
  await page.getByLabel('Feature name').fill('Search');
  await shot('promote-form');
  await page.getByRole('button', { name: 'Promote' }).last().click();
  await working();
  const panel = page.getByRole('region', { name: 'Proposal' });
  await panel.waitFor({ timeout: 240000 });
  await shot('proposal');
  console.log(`proposal:\n${await panel.innerText()}`);
  check(true, 'promote turn ends with a proposal panel');
  await page.waitForTimeout(1000);
  await shot('checkpoints-after-promote');
  const left = await rows.allInnerTexts();
  check(
    !left.some((t) => quick.some((id) => t.includes(`Before turn ${id} `))),
    'the quick-session checkpoints, taken in another tree, have no Rollback button after the promote',
    `rows: ${JSON.stringify(left)}; GET rollbackable: ${JSON.stringify((await getJson(`/turns?channel=${cid}&rollbackable=1`)).map((t) => t.id))}`,
  );
  const under = section
    .locator('li', { has: page.getByRole('link', { name: 'Search', exact: true }) })
    .getByRole('link', { name: 'idea chat' });
  await under.waitFor();
  check(
    (await section.getByRole('heading', { name: 'Quick sessions' }).count()) === 0,
    'sidebar moves the channel under the new feature',
    `channel row: ${sql(`select id,feature_id from channel where id=${cid}`)}`,
  );
  await panel.getByRole('button', { name: 'Approve' }).click();
  await page.getByText(/approved: \d+ decisions added/).waitFor();
  await shot('approved');
  await section.getByRole('link', { name: 'Search', exact: true }).click();
  await page.getByText('Version 1 by').waitFor();
  await shot('feature-page');
  const where = `feature_id=(select feature_id from channel where id=${cid})`;
  const count = sql(`select count(*) from brief where ${where}`);
  check(
    count === '1',
    'brief version 1 on the feature page and in the database',
    `brief rows: ${count}, author: ${sql(`select author from brief where ${where}`)}`,
  );
}

async function cap() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'cap chat');
  await exchange('Say hello.');
  sql(`update channel set token_budget=1 where id=${cid}`, true);
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
  const state = sql(`select token_budget,paused from channel where id=${cid}`);
  const lead = sql(`select count(*) from message where channel_id=${cid} and author='lead'`);
  check(lead === '2', 'next message ran after Continue', `channel row (budget|paused): ${state}; lead messages: ${lead}`);
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
  await addFeature(section, 'login', 'Login');
  const fid = page.url().split('/').pop();
  await main.getByLabel('Stage').selectOption('live-testing');
  await main.locator('[aria-current="step"]', { hasText: 'Live testing' }).waitFor();
  await page.reload();
  const bar = await main.locator('[aria-current="step"]').innerText();
  const stage = await main.getByLabel('Stage').inputValue();
  check(stage === 'live-testing' && bar === 'Live testing', 'stage survives a reload', `select ${stage}, bar ${bar}, row ${sql(`select stage from feature where id=${fid}`)}`);
  await page.goto(base + '/tasks');
  const title = `ui task ${Date.now().toString(36)}`;
  await page.getByLabel('Task title').fill(title);
  const pid = sql(`select project_id from feature where id=${fid}`);
  await page.getByLabel('Task project').selectOption(pid);
  await page.getByLabel('Task feature').selectOption(fid);
  await page.getByRole('button', { name: 'Add task' }).click();
  const row = main.locator('li', { hasText: title });
  await row.waitFor();
  await page.getByLabel('Filter by project').selectOption(pid);
  await page.getByLabel('Filter by feature').selectOption(fid);
  await row.getByRole('checkbox').click();
  await row.getByRole('checkbox', { checked: true }).waitFor();
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  await row.waitFor();
  await shot('tasks-done');
  check(
    sql(`select status from task where title='${title}'`) === 'done' && page.url().includes('status=done'),
    'a task added in the window is marked done and filtered',
    `row ${sql(`select id,project_id,feature_id,status from task where title='${title}'`)}; url ${page.url().replace(base, '')}`,
  );
}

async function capture() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  await page.goto(base + '/capture');
  await page.getByLabel('Capture project').selectOption({ label: name });
  const text = `ui capture ${Date.now().toString(36)}`;
  await page.getByLabel('Capture text').fill(text);
  await page.getByLabel('Capture text').press('Enter');
  await page.getByText('Captured').waitFor({ timeout: 15000 });
  await shot('captured');
  check((await page.getByLabel('Capture text').inputValue()) === '', 'the text box is empty after capture');
  const row = sql(`select title from task where project_id=${pid} and title='todo: ${text}'`);
  check(row === `todo: ${text}`, 'the captured text is stored as a task row', row);

  // A failing request shows its error: the picked project is deleted before Enter.
  const section2 = await addProject();
  const name2 = (await section2.getAttribute('aria-label')) ?? '';
  const pid2 = sql(`select id from project where name='${name2}'`);
  await page.goto(base + '/capture');
  await page.getByLabel('Capture project').selectOption({ label: name2 });
  sql(`delete from project where id=${pid2}`, true);
  await page.getByLabel('Capture text').fill('should fail');
  await page.getByLabel('Capture text').press('Enter');
  const err = page.getByText(/unknown project/);
  await err.waitFor({ timeout: 15000 });
  await shot('error');
  check(true, 'a capture whose request fails shows the error', await err.innerText());
}

async function stop() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'stop chat');
  await send('Remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools.');
  await working();
  await main.locator('div.border-dashed p.whitespace-pre-wrap').first().waitFor({ timeout: 60000 });
  await page.waitForTimeout(1000);
  await page.getByRole('button', { name: 'Stop' }).click();
  await page.getByText(/Failed: interrupted/).waitFor({ timeout: 30000 });
  await shot('stopped');
  check(true, 'Stop shows the turn failed', sql(`select id,status,error from turn where channel_id=${cid}`));
  const partial = sql(`select substr(replace(body, char(10), ' '),1,40) from message where channel_id=${cid} and author='lead'`);
  const shown = await main.getByText(partial, { exact: false }).count();
  check(Boolean(partial) && shown === 1, 'the partial reply shows once, not as the message and as the live block', `partial: ${JSON.stringify(partial)}; shown ${shown} times`);
  await exchange('What was the secret word? Answer with the word only.');
  await shot('resumed');
  const answer = sql(`select body from message where channel_id=${cid} and author='lead' order by id desc limit 1`);
  check(/pineapple/i.test(answer), 'the next message resumes the session', `answer: ${answer}`);
}

async function retry() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'retry chat');
  await send('Write a 1500-word story about lighthouses. Use no tools.');
  await working();
  await page.waitForTimeout(4000);
  await page.getByRole('button', { name: 'Stop' }).click();
  const row = page.getByRole('listitem').filter({ hasText: /Turn \d+ by lead failed: interrupted/ });
  await row.waitFor({ timeout: 30000 });
  await shot('failed-row');
  await row.getByRole('button', { name: 'Retry' }).click();
  await working();
  await shot('retrying');
  await idle();
  await row.waitFor({ state: 'hidden' });
  await shot('retried');
  const turns = sql(`select id||' '||role||' '||status||' retried='||retried from turn where channel_id=${cid} order by id`);
  check(/^\d+ lead failed retried=1\n\d+ lead done retried=0$/.test(turns), 'Retry runs the failed wake as a new turn that ends done', turns);
}

const git = (dir, ...args) =>
  execFileSync('git', ['-C', dir, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
function ensureRepo() {
  try {
    git(projectDir, 'rev-parse', 'HEAD');
  } catch {
    git(projectDir, 'init', '-q', '-b', 'main');
    writeFileSync(path.join(projectDir, 'README.md'), 'hello\n');
    git(projectDir, 'add', '.');
    git(projectDir, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'first');
  }
}

async function rollback() {
  ensureRepo();
  const section = await addProject();
  const slug = await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'rollback chat');
  const tree = path.join(data, 'worktrees', sql(`select project_id from channel where id=${cid}`), slug);
  const state = () => [git(tree, 'rev-parse', 'HEAD'), git(tree, 'status', '--porcelain'), git(tree, 'log', '--oneline'), git(tree, 'ls-files')].join('\n--\n');

  await exchange('Say hello.');
  writeFileSync(path.join(tree, 'mine.txt'), 'my notes\n');
  const before = state();
  await send('Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.');
  await working();
  await idle();
  git(tree, 'add', '-A');
  git(tree, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'by the turn');
  const turnId = sql(`select id from turn where channel_id=${cid} order by id desc limit 1`);
  const list = main.getByRole('region', { name: 'Checkpoints' });
  await list.getByText(`Before turn ${turnId} by lead`).waitFor({ timeout: 15000 });
  await shot('checkpoints');
  check(
    git(tree, 'log', '--oneline').includes('by the turn') && (await list.getByRole('button', { name: 'Rollback' }).count()) === 2,
    'two turns have a Rollback button and the turn left a commit and new.txt',
    `files: ${git(tree, 'ls-files')}; log:\n${git(tree, 'log', '--oneline')}`,
  );
  let asked = '';
  page.once('dialog', (d) => {
    asked = d.message();
    d.accept();
  });
  await list.getByRole('listitem').filter({ hasText: `Before turn ${turnId} by` }).getByRole('button', { name: 'Rollback' }).click();
  await page.getByText(`Rolled back to before turn ${turnId}.`).waitFor({ timeout: 15000 });
  await shot('rolled-back');
  check(asked.includes(`Roll back to before turn ${turnId}?`), 'Rollback asks to confirm', asked);
  const after = state();
  check(after === before, 'HEAD, status, log and files are as before the turn', `before:\n${before}\nafter:\n${after}`);
  check(
    !existsSync(path.join(tree, 'new.txt')) && existsSync(path.join(tree, 'mine.txt')),
    'new.txt is gone and mine.txt is back',
    `ls: ${readdirSync(tree).filter((f) => f !== '.git').join(' ')}`,
  );
  check(
    sql(`select count(*) from message where channel_id=${cid} and author='reef' and body='Rolled back to before turn ${turnId}.'`) === '1',
    'the channel has the note from reef',
  );
  await exchange('Did anything change in the files since your last answer? Answer in one sentence.');
  // The note is added to the prompt when it is sent, so it is in claude's session file, not in wake_text.
  const sid = sql(`select session_id from turn where channel_id=${cid} order by id desc limit 1`);
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
  await addFeature(section, 'login', 'Login');
  const featureId = page.url().split('/').pop();
  const cid = await newChannel(main, 'New channel', 'diff chat');

  // Open the diff panel before the turn runs, and leave it open: a turn started through the
  // API (not the channel page) should still land on this still-mounted panel with no reload.
  await page.goto(`${base}/features/${featureId}`);
  const panel = main.locator('section', { has: page.getByRole('heading', { name: 'Diff', level: 2 }) });
  await panel.getByText('No changes.').waitFor({ timeout: 15000 });
  await shot('empty');
  await fetch(`${base}/channels/${cid}/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: 'Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.' }),
  });
  await panel.getByText('new.txt').waitFor({ timeout: 30000 });
  await shot('diff');
  check(
    (await panel.getByText('added').count()) === 1 && (await panel.getByText('hi').count()) === 1,
    'the diff panel shows new.txt as added with its line, open since before the turn and never reloaded',
    await panel.innerText(),
  );
  const json = await getJson(`/features/${featureId}/diff`);
  check(
    json.length === 1 && json[0].path === 'new.txt' && json[0].status === 'added' && json[0].hunks[0].lines[0].text === 'hi',
    'GET /features/{id}/diff returns the file with its added line',
    JSON.stringify(json),
  );
}

async function memory() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  const home = main.locator('section', { has: page.getByRole('heading', { name, level: 2 }) });
  await home.getByLabel('Project memory').waitFor();
  await shot('empty');
  check((await home.getByLabel('Project memory').inputValue()) === '', 'a new project has no memory yet');
  await home.getByLabel('Project memory').fill('Use Go, not Python.');
  await home.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  await shot('saved');
  check(
    sql(`select memory from project where id=${pid}`) === 'Use Go, not Python.',
    'saving the editor stores the memory',
    sql(`select id,memory from project where id=${pid}`),
  );
  await page.reload();
  const value = await home.getByLabel('Project memory').inputValue();
  check(value === 'Use Go, not Python.', 'the saved memory survives a reload', value);
}

async function inspector() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const { id: cid } = await newTeamChannel('full', 'inspector room');
  await send('Say ok in one word.');
  await workingAs('lead');
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.');
  await workingAs('developer');
  await idle();

  await main.getByRole('button', { name: 'Inspector' }).click();
  const panel = page.getByRole('complementary', { name: 'Inspector' });
  const devSummary = panel.locator('summary', { hasText: 'developer' });
  await devSummary.waitFor({ timeout: 15000 });
  await shot('panel');

  const json = await getJson(`/channels/${cid}/inspector`);
  const leadRole = json.roles.find((r) => r.role === 'lead');
  const devRole = json.roles.find((r) => r.role === 'developer');
  check(!!leadRole && !!devRole, 'the API lists a lead role and a developer role', JSON.stringify(json.roles));

  const turnsLabel = (n) => `${n} turn${n === 1 ? '' : 's'}`;
  const leadText = await panel.locator('summary', { hasText: 'lead' }).innerText();
  const devText = await devSummary.innerText();
  check(
    leadText.includes(turnsLabel(leadRole.turns)) && leadText.includes(`${leadRole.usage.inputTokens} in`),
    'the lead block shows turn count and tokens from the API',
    leadText,
  );
  check(
    devText.includes(turnsLabel(devRole.turns)) && devText.includes(`${devRole.usage.inputTokens} in`),
    'the developer block shows turn count and tokens from the API',
    devText,
  );

  await devSummary.click();
  const devTurn = json.turns.find((t) => t.role === 'developer');
  await panel.locator('details summary').last().click();
  await shot('turn-open');
  const body = await panel.innerText();
  check(body.includes(devTurn.prompt), 'expanding the developer turn shows its prompt', body);
  check(
    devTurn.toolCalls.length > 0 && body.includes(devTurn.toolCalls[0].name) && body.includes(devTurn.toolCalls[0].result),
    'the developer turn shows a tool call with its input and result',
    body,
  );
}

async function review() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) {
    throw new Error('apply the team full first (teams/full.json, see features/teams.md)');
  }
  ensureRepo();
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const featureId = page.url().split('/').pop();
  const { id: cid } = await newTeamChannel('full', 'review room');
  await send('@developer Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.');
  await workingAs('developer', 60000);
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes.');
  await workingAs('developer', 60000);
  await idle();
  await send('@developer Use the Bash tool to delete README.md. Then reply done.');
  await workingAs('developer', 60000);
  await idle();
  const root = (await getJson(`/messages?channel=${cid}`))
    .filter((m) => m.author === 'developer' && m.parentId === null)
    .at(-1);
  check(!!root, 'the developer posted a third top-level message, its latest thread root', root && `message ${root.id}`);

  await page.goto(`${base}/features/${featureId}`);
  const panel = main.locator('section', { has: page.getByRole('heading', { name: 'Diff', level: 2 }) });
  await panel.getByText('README.md').waitFor({ timeout: 15000 });
  await shot('diff');
  await panel.locator('[data-path="new.txt"][data-line="1"]').click();
  await panel.getByLabel('Comment on new.txt:1').fill('nice start');
  await panel.getByRole('button', { name: 'Add comment' }).click();
  await panel.locator('[data-path="d.txt"][data-line="1"]').click();
  await panel.getByLabel('Comment on d.txt:1').fill('rename this');
  await panel.getByRole('button', { name: 'Add comment' }).click();
  await panel.locator('[data-path="README.md"][data-line="1"]').click();
  await panel.getByLabel('Comment on README.md:1 (removed)').fill('was this needed?');
  await panel.getByRole('button', { name: 'Add comment' }).click();
  await shot('comments');
  const before = Number(sql(`select count(*) from turn where channel_id=${cid} and role='developer'`));
  await panel.getByRole('button', { name: 'Send to developer' }).click();
  // The review message matches no recorded scenario, so the turn may error and end before
  // the window ever shows "is working...": poll the turn row instead of workingAs/idle.
  let after = before;
  for (let i = 0; i < 60 && after === before; i++) {
    await page.waitForTimeout(500);
    after = Number(sql(`select count(*) from turn where channel_id=${cid} and role='developer'`));
  }
  await idle();
  await shot('sent');

  const sent = (await getJson(`/messages?channel=${cid}`)).at(-1);
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
    (await panel.getByRole('button', { name: 'Send to developer' }).count()) === 0,
    'comments clear after a successful send',
  );
}

async function inbox() {
  if ((await fetch(`${base}/teams/gated`)).status !== 200) throw new Error('apply the team gated first (see features/teams.md)');
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
      const item = (await getJson('/inbox')).find((i) => i.kind === 'waiting' && String(i.channelId) === String(cid));
      if (item && item.at !== since) return item;
      await page.waitForTimeout(1000);
    }
    throw new Error('no new waiting item');
  };
  const sendApi = (cid, text) =>
    fetch(`${base}/channels/${cid}/send`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }) });
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const { id: cid } = await newTeamChannel('gated', 'inbox room');
  const permission = await page.evaluate(() => Notification.permission);
  check(permission === 'granted', 'the window may show notifications', permission);

  await send('DESIGN a short plan for a login form.');
  await workingAs('lead', 60000);
  await aside.getByRole('link', { name: /^Needs you/ }).click();
  await page.getByRole('heading', { name: 'Needs you', level: 1 }).waitFor();
  check(page.url().endsWith('/inbox'), 'the Needs you link opens /inbox', page.url());
  const row = main.getByRole('listitem').filter({ hasText: 'lead wants to wake architect' });
  await row.waitFor({ timeout: 240000 });
  await shot('approval-row');
  const rowText = (await row.textContent()) ?? '';
  check(
    /Approval/.test(rowText) && /# inbox room/.test(rowText) && (await row.getByRole('button', { name: 'Approve' }).count()) === 1 && (await row.getByRole('button', { name: 'Reject' }).count()) === 1,
    'a new approval of a channel that is not open shows in the inbox with Approve and Reject',
    rowText,
  );
  const badge = await aside.getByLabel(/waiting$/).textContent();
  check(badge === '1', 'the Inbox link shows a count badge of 1', badge);
  await page.waitForTimeout(3000);
  const fired = await notes();
  check(fired.length === 1 && /^Approval: inbox room$/.test(fired[0].title) && fired[0].body === 'lead wants to wake architect', 'one notification fired for the approval', JSON.stringify(fired));
  check(sql(`select count(*) from turn where channel_id=${cid} and role='architect'`) === '0', 'no architect turn ran before Approve');

  const aid = sql(`select id from approval where channel_id=${cid}`);
  await page.evaluate(() => window.__notes[0].note.onclick());
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  let landed = await landedOn(`approval-${aid}`);
  check(page.url().endsWith(`/channels/${cid}#approval-${aid}`) && landed.inView && landed.ringed, 'clicking the notification opens the channel at the approval row, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await aside.getByRole('link', { name: /^Needs you/ }).click();
  await row.getByRole('link', { name: 'Open channel' }).click();
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  landed = await landedOn(`approval-${aid}`);
  check(page.url().endsWith(`/channels/${cid}#approval-${aid}`) && landed.inView && landed.ringed, 'clicking an inbox item opens its channel at the approval row, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await shot('item-opened-channel');

  await aside.getByRole('link', { name: /^Needs you/ }).click();
  await row.getByRole('button', { name: 'Approve' }).click();
  await row.waitFor({ state: 'hidden', timeout: 30000 });
  await shot('approved');
  check(true, 'Approve from the inbox removes the approval item');
  const end = Date.now() + 240000;
  while (Date.now() < end && sql(`select count(*) from turn where channel_id=${cid} and role='architect' and status='done'`) === '0') await page.waitForTimeout(1000);
  check(
    sql(`select from_role||'>'||to_role||' '||status from approval where channel_id=${cid}`) === 'lead>architect approved' &&
      sql(`select count(*) from turn where channel_id=${cid} and role='architect' and status='done'`) === '1',
    'Approve ran the wake: the approval is approved and the architect turn is done',
    `approvals: ${sql(`select id,status from approval where channel_id=${cid}`)}; turns: ${sql(`select group_concat(role||' '||status) from (select role,status from turn where channel_id=${cid} order by id)`)}`,
  );
  const waiting = main.getByRole('listitem').filter({ hasText: /Waiting.*inbox room/ });
  await waiting.waitFor({ timeout: 30000 });
  await shot('after-wake');
  check((await waiting.getByRole('link', { name: 'Open' }).count()) === 1, 'the architect answer then shows as a waiting item with Open', await waiting.textContent());
  const all = await notes();
  check(all.length === 2 && /^Waiting: inbox room$/.test(all[1].title), 'the new item fired its own notification, none for the old ones', JSON.stringify(all.map((n) => n.title)));
  const before = (await getJson('/inbox')).find((i) => i.kind === 'waiting').at;
  await waiting.getByRole('link', { name: 'Open' }).click();
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  landed = await landedOn('latest-message');
  check(page.url().endsWith(`/channels/${cid}#latest-message`) && landed.inView && landed.ringed, 'Open on a waiting item opens the channel at the newest message, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await shot('waiting-opened-channel');

  await sendApi(cid, 'Say ok in one word. Use no tools.');
  const visible = await waitWaiting(cid, before);
  await page.waitForTimeout(3000);
  const whileOpen = await notes();
  check(whileOpen.length === 2, 'a new item of the open, visible channel fires no notification', JSON.stringify({ item: visible.text, notes: whileOpen.map((n) => n.title) }));

  await page.evaluate(() => Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' }));
  await sendApi(cid, 'Say fine in one word. Use no tools.');
  await waitWaiting(cid, visible.at);
  await page.waitForTimeout(3000);
  const whileHidden = await notes();
  check(whileHidden.length === 3 && /^Waiting: inbox room$/.test(whileHidden[2].title), 'the same channel hidden from view fires one notification', JSON.stringify(whileHidden.map((n) => n.title)));

  await page.goto(base + '/inbox');
  await waiting.waitFor();
  await page.waitForTimeout(3000);
  const afterReload = await notes();
  check(afterReload.length === 0, 'items present at load fire no notification', JSON.stringify(afterReload));
}

async function wrap() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'wrap chat');
  await exchange('We are building email login. Decision: sessions, not JWT, because there is one server.');
  await page.getByRole('button', { name: 'Wrap up' }).click();
  await working();
  const panel = page.getByRole('region', { name: 'Proposal' });
  await panel.waitFor({ timeout: 240000 });
  await shot('proposal');
  await panel.getByRole('button', { name: 'Reject' }).click();
  await page.getByText('rejected', { exact: true }).waitFor();
  const fid = sql(`select feature_id from channel where id=${cid}`);
  check(
    sql(`select status from proposal where channel_id=${cid} order by id desc limit 1`) === 'rejected' &&
      sql(`select count(*) from brief where feature_id=${fid}`) === '0',
    'Reject leaves the brief unchanged',
    `proposals ${sql(`select id,status from proposal where channel_id=${cid}`)}`,
  );
  await send('/wrap');
  await working();
  await panel.waitFor({ timeout: 240000 });
  await panel.getByRole('button', { name: 'Approve' }).click();
  await page.getByText(/approved: \d+ decisions added/).waitFor();
  await shot('approved');
  await section.getByRole('link', { name: 'Login', exact: true }).click();
  await page.getByText('Version 1 by').waitFor();
  check(sql(`select count(*) from brief where feature_id=${fid}`) === '1', '/wrap then Approve makes brief version 1', `brief rows ${sql(`select id,author from brief where feature_id=${fid}`)}`);
}

async function teams() {
  const missing = [];
  for (const name of ['duo', 'gated']) if ((await fetch(`${base}/teams/${name}`)).status !== 200) missing.push(name);
  if (missing.length) throw new Error(`apply the teams first: missing ${missing.join(', ')} (see features/teams.md)`);
  const section = await addProject();
  await addFeature(section, 'login', 'Login');

  const { id: cid, options } = await newTeamChannel('duo', 'duo room', 'team-picked');
  check(options.includes('duo') && options.includes('gated') && options.includes('solo'), 'the New channel form lists the teams', options.join(', '));
  const channel = await getJson(`/channels/${cid}`);
  check(channel.team === 'duo' && channel.role === 'lead', 'the channel is created on team duo', JSON.stringify(channel));

  await send('DESIGN a short plan for a login form.');
  const leadSeen = await workingAs('lead', 60000);
  await shot('lead-working');
  check(leadSeen, 'the live turn names the lead while it runs');
  const architectSeen = await workingAs('architect');
  await shot('architect-working');
  check(architectSeen, 'then the live turn names the architect');
  await rootRow('architect').first().waitFor({ timeout: 240000 });
  await idle();
  const root = (await getJson(`/messages?channel=${cid}`)).find((m) => m.author === 'architect' && m.parentId === null);
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
  const authors = await pane.locator('li > div').allTextContents();
  check(authors.join(',') === 'architect,user,architect', 'the Thread pane shows the root, the user reply and the architect answer', authors.join(','));
  const inThread = (await getJson(`/messages?channel=${cid}`)).filter((m) => m.parentId === root.id).map((m) => m.author);
  check(inThread.join(',') === 'user,architect', 'both replies are stored under the root', inThread.join(','));
  await pane.getByRole('button', { name: 'Close' }).click();
  const count = await rootRow('architect').last().getByRole('button').textContent();
  check(count === '2 replies in thread', 'the main list shows the root with a replies button', count);
  await shot('main-list');

  await section.getByRole('link', { name: 'Login', exact: true }).click();
  await page.getByRole('heading', { name: 'Login', level: 1 }).waitFor();
  const { id: gid } = await newTeamChannel('gated', 'gate room');
  await send('DESIGN a password reset form.');
  const approve = main.getByRole('button', { name: 'Approve' });
  await approve.waitFor({ timeout: 240000 });
  await idle();
  const row = (await approve.locator('xpath=ancestor::li[1]').textContent()) ?? '';
  check(
    /lead → architect:/.test(row) && (await main.getByRole('button', { name: 'Reject' }).count()) === 1,
    'the gated channel shows a lead → architect row with Approve and Reject',
    row,
  );
  check(sql(`select count(*) from turn where channel_id=${gid} and role='architect'`) === '0', 'no architect turn ran before Approve');
  await shot('gate-approval');
  await approve.click();
  const approvedSeen = await workingAs('architect', 120000);
  await shot('gate-approved');
  check(approvedSeen, 'Approve runs the architect');
  await rootRow('architect').first().waitFor({ timeout: 240000 });
  await idle();
  check((await main.getByRole('button', { name: 'Approve' }).count()) === 0, 'the approval row is gone');
  await shot('gate-architect-posted');
  check(
    sql(`select from_role||'>'||to_role||' '||status from approval where channel_id=${gid}`) === 'lead>architect approved',
    'the approval is stored as approved',
    `approvals: ${sql(`select id,from_role,to_role,status from approval where channel_id=${gid}`)}; turns: ${sql(`select group_concat(role) from (select role from turn where channel_id=${gid} order by id)`)}`,
  );
}

// Dispatches a native HTML5 drag sequence (dragstart on `card`, dragover and drop on
// `target`) through the DOM, since the window uses the draggable attribute, not mouse
// gestures a synthetic drag can trigger headless.
async function dragCard(card, target) {
  const cardEl = await card.elementHandle();
  const targetEl = await target.elementHandle();
  await page.evaluate(
    ([c, t]) => {
      const data = new DataTransfer();
      c.dispatchEvent(new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer: data }));
      t.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer: data }));
      t.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: data }));
    },
    [cardEl, targetEl],
  );
}

async function board() {
  const section = await addProject();
  await addFeature(section, 'board', 'Board feature');
  const fid = page.url().split('/').pop();
  const pid = sql(`select project_id from feature where id=${fid}`);

  const otherSection = await addProject();
  await addFeature(otherSection, 'other', 'Other feature');
  const otherFid = page.url().split('/').pop();
  const otherPid = sql(`select project_id from feature where id=${otherFid}`);

  await page.goto(base + '/tasks');
  const title = `board task ${Date.now().toString(36)}`;
  await page.getByLabel('Task title').fill(title);
  await page.getByLabel('Task project').selectOption(pid);
  await page.getByLabel('Task feature').selectOption(fid);
  await page.getByRole('button', { name: 'Add task' }).click();
  await page.getByText(title).waitFor();

  const otherTitle = `board other ${Date.now().toString(36)}`;
  await page.getByLabel('Task title').fill(otherTitle);
  await page.getByLabel('Task project').selectOption(otherPid);
  await page.getByLabel('Task feature').selectOption(otherFid);
  await page.getByRole('button', { name: 'Add task' }).click();
  await page.getByText(otherTitle).waitFor();

  await page.getByRole('button', { name: 'Board' }).click();
  const todo = page.getByRole('group', { name: 'Todo' });
  const doing = page.getByRole('group', { name: 'Doing' });
  const done = page.getByRole('group', { name: 'Done' });
  await todo.getByText(title).waitFor();
  await shot('columns');
  check(
    page.url().includes('view=board') &&
      (await todo.getByText(title).count()) === 1 &&
      (await todo.getByText(otherTitle).count()) === 1 &&
      (await doing.count()) === 1 &&
      (await done.count()) === 1,
    '/tasks?view=board shows three columns with both tasks under Todo',
    page.url().replace(base, ''),
  );

  await page.getByLabel('Filter by project').selectOption(pid);
  await todo.getByText(otherTitle).waitFor({ state: 'hidden' });
  check(
    (await todo.getByText(title).count()) === 1 && (await todo.getByText(otherTitle).count()) === 0,
    'the project filter also filters the board',
    page.url().replace(base, ''),
  );
  await page.getByLabel('Filter by feature').selectOption(fid);
  await todo.getByText(title).waitFor();
  check((await todo.getByText(title).count()) === 1, 'the feature filter also filters the board', page.url().replace(base, ''));

  const card = todo.locator('article', { hasText: title });
  await dragCard(card, doing);
  await doing.getByText(title).waitFor();
  await shot('dragged');
  check(
    sql(`select status from task where title='${title}'`) === 'doing',
    'dragging the card to Doing changes its row in SQLite',
    sql(`select id,status from task where title='${title}'`),
  );
  await page.reload();
  await page.getByLabel('Filter by project').selectOption(pid);
  await page.getByLabel('Filter by feature').selectOption(fid);
  await doing.getByText(title).waitFor();
  check((await doing.getByText(title).count()) === 1, 'a reload keeps the card in Doing', page.url().replace(base, ''));

  await doing.getByLabel(`Move ${title}`).selectOption('done');
  await done.getByText(title).waitFor();
  await shot('moved-with-select');
  check(
    sql(`select status from task where title='${title}'`) === 'done',
    'the per-card select moves a card for keyboard use',
    sql(`select id,status from task where title='${title}'`),
  );
}

// Waits for text matching pattern to show up in `main`, and returns it, or null on timeout:
// an invalid save's error does not abort the scenario.
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
  sql(`update role set mcp_servers = '${mcpBefore}' where team_id = (select id from team where name = 'full') and name = 'developer'`, true);

  await page.goto(`${base}/teams`);
  await main.getByRole('button', { name: 'full', exact: true }).click();
  const personaBox = main.getByLabel('developer persona');
  await personaBox.waitFor();
  const marker = `WORKSHOP-MARKER-${Date.now().toString(36)}`;
  const persona = `${await personaBox.inputValue()} Always say ${marker} once.`;
  await personaBox.fill(persona);
  await shot('persona-edited');
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  await shot('persona-saved');
  check(
    sql(`select persona from role where team_id = (select id from team where name = 'full') and name = 'developer'`) === persona,
    "saving the developer's persona in the window updates its role row",
  );
  check(
    sql(`select mcp_servers from role where team_id = (select id from team where name = 'full') and name = 'developer'`) === mcpBefore,
    "the developer's mcpServers, left out of the window, are unchanged",
  );

  // Two leads: the same error reef team apply gives.
  await main.getByLabel('architect lead').check();
  await main.getByRole('button', { name: 'Save' }).click();
  const twoLeads = await errorText(/needs exactly one lead role, has 2/);
  await shot('two-leads-error');
  check(!!twoLeads, 'an invalid team (two leads) shows the reef team apply error', twoLeads ?? '(not shown)');
  await main.getByLabel('architect lead').uncheck();

  // Unknown backend: the same error reef team apply gives.
  await main.getByLabel('architect backend').fill('gemini');
  await main.getByRole('button', { name: 'Save' }).click();
  const badBackend = await errorText(/backend must be claude or codex/);
  await shot('bad-backend-error');
  check(!!badBackend, 'an invalid team (unknown backend) shows the reef team apply error', badBackend ?? '(not shown)');
  await main.getByLabel('architect backend').fill('claude');
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();

  // "Copy as new team" makes a new team `reef team list` (the same store) shows.
  const copyName = `workshop-copy-${Date.now().toString(36)}`;
  await main.getByLabel('New team name').fill(copyName);
  await main.getByRole('button', { name: 'Copy as new team' }).click();
  await main.getByRole('button', { name: copyName, exact: true }).waitFor({ timeout: 15000 });
  await shot('copied');
  check(
    sql(`select count(*) from team where name = '${copyName}'`) === '1',
    '"Copy as new team" makes a new team',
  );

  // The next developer wake uses the new persona.
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const { id: cid } = await newTeamChannel('full', 'workshop room');
  await send('@developer Say ok in one word.');
  await workingAs('developer');
  await idle();
  const prompt = sql(`select prompt from turn where channel_id = ${cid} and role = 'developer' order by id limit 1`);
  check(prompt.includes(marker), "the next developer wake's prompt carries the new persona", prompt.slice(0, 200));
}

try {
  await scenarios[scenario]();
} catch (err) {
  check(false, 'scenario aborted', String(err.message).split('\n')[0]);
  await shot('error').catch(() => {});
}
await browser.close();
process.exit(failed ? 1 : 0);

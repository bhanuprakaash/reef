// Drives the Reef window in Chrome and prints PASS/FAIL lines. See ../../SKILL.md.
// usage: node drive.mjs live|brief|promote|cap|pages|stop|wrap BASE_URL
// env: REEF_UI_DATA (data folder of the server, for sqlite3), REEF_UI_PROJECT (project
// folder, default a new temp dir), REEF_UI_PROOF (screenshot folder, default ./proof),
// REEF_VERIFY_CHROME (path of a Chrome or Chromium binary; default the system Chrome).
import { chromium } from 'playwright-core';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const [scenario, base] = process.argv.slice(2);
const scenarios = { live, brief, promote, cap, pages, stop, wrap };
if (!scenarios[scenario] || !base) {
  console.error('usage: node drive.mjs live|brief|promote|cap|pages|stop|wrap BASE_URL');
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
const aside = page.locator('aside');
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

// Adds a project through the form; returns its sidebar section.
async function addProject() {
  const name = `ui-${scenario}-${Date.now().toString(36)}`;
  await page.goto(base + '/');
  await page.getByRole('button', { name: 'Add project' }).click();
  await page.getByLabel('Project name').fill(name);
  await page.getByLabel('Project path').fill(projectDir);
  await page.getByRole('button', { name: 'Add', exact: true }).click();
  const section = aside.locator('section', { has: page.getByRole('heading', { name, level: 3 }) });
  await section.waitFor();
  return section;
}
async function addFeature(section, slug, name) {
  await section.getByRole('button', { name: 'Add feature' }).click();
  await section.getByLabel('Feature slug').fill(slug);
  await section.getByLabel('Feature name').fill(name);
  await section.getByRole('button', { name: 'Add', exact: true }).click();
  await section.getByRole('link', { name, exact: true }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
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
  const section = await addProject();
  const cid = await newChannel(section, 'New quick session', 'idea chat');
  await exchange('I have an idea: a search bar for the shop that filters products as you type.');
  await exchange('Decision: search is client side only for now, because the catalog is small (under 200 items).');
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
  await page.getByRole('button', { name: 'Add project' }).click();
  await page.getByLabel('Project name').fill('bad path');
  await page.getByLabel('Project path').fill('/nonexistent-reef-ui');
  await page.getByRole('button', { name: 'Add', exact: true }).click();
  const err = page.getByText(/not a directory/);
  await err.waitFor({ timeout: 10000 });
  await shot('bad-path');
  check(true, 'a bad project path shows the server error', await err.innerText());
  await page.reload();
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const fid = page.url().split('/').pop();
  await main.getByRole('combobox').first().selectOption('live-testing');
  await main.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved').waitFor();
  await page.reload();
  const stage = await main.getByRole('combobox').first().inputValue();
  check(stage === 'live-testing', 'stage survives a reload', `select ${stage}, row ${sql(`select stage from feature where id=${fid}`)}`);
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
  await row.getByRole('button', { name: 'Mark done' }).click();
  await row.getByRole('button', { name: 'Mark done' }).waitFor({ state: 'hidden' });
  await page.getByLabel('Filter by status').selectOption('done');
  await row.waitFor();
  await shot('tasks-done');
  check(
    sql(`select status from task where title='${title}'`) === 'done' && page.url().includes('status=done'),
    'a task added in the window is marked done and filtered',
    `row ${sql(`select id,project_id,feature_id,status from task where title='${title}'`)}; url ${page.url().replace(base, '')}`,
  );
}

async function stop() {
  const section = await addProject();
  await addFeature(section, 'login', 'Login');
  const cid = await newChannel(main, 'New channel', 'stop chat');
  await send('Remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools.');
  await working();
  await page.waitForTimeout(4000);
  await page.getByRole('button', { name: 'Stop' }).click();
  await page.getByText(/Failed: interrupted/).waitFor({ timeout: 30000 });
  await shot('stopped');
  check(true, 'Stop shows the turn failed', sql(`select id,status,error from turn where channel_id=${cid}`));
  await exchange('What was the secret word? Answer with the word only.');
  await shot('resumed');
  const answer = sql(`select body from message where channel_id=${cid} and author='lead' order by id desc limit 1`);
  check(/pineapple/i.test(answer), 'the next message resumes the session', `answer: ${answer}`);
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

try {
  await scenarios[scenario]();
} catch (err) {
  check(false, 'scenario aborted', String(err.message).split('\n')[0]);
  await shot('error').catch(() => {});
}
await browser.close();
process.exit(failed ? 1 : 0);

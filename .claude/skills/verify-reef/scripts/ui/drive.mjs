// Drives the Reef window in Chrome and prints PASS/FAIL lines. See ../../SKILL.md.
// usage: node drive.mjs live|brief|attach|cap|pages|stop|wrap|teams|team-management|retry|rollback|inbox|inbox-groups|diff|memory|inspector|review|board|workshop|skills|capture|theme|phone|pr|coordination|team|denials|subagent|composer|naming|conversation|conversation-move|hovercard|mention|providers|bots|work-item|work-item-review|first-run|creatures|bot|space-header|sidebar|global-chats|navigate|pending-memory BASE_URL
// env: REEF_UI_DATA (data folder of the server, for sqlite3), REEF_UI_PROJECT (project
// folder, default a new temp dir), REEF_UI_PROOF (screenshot folder, default ./proof),
// REEF_VERIFY_CHROME (path of a Chrome or Chromium binary; default the system Chrome),
// REEF_FAKE_GH (pr only: the folder the server's fake gh keeps its PR in, see fake-gh/gh).
// REEF_FAKE_LOG (denials and naming only: the fake claude's log, one line per turn with its args).
// REEF_UI_REEF (memory and bot only: the reef binary, to run reef memory add and recall).
// REEF_UI_FAKE (providers only: the folder holding the fake claude, used as a second install).
import { chromium } from 'playwright-core';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import path from 'node:path';

const [scenario, base] = process.argv.slice(2);
const scenarios = { live, 'pending-memory': pendingMemory, brief, attach, cap, pages, stop, wrap, teams, 'team-management': teamManagement, retry, rollback, inbox, 'inbox-groups': inboxGroups, diff, memory, inspector, review, board, workshop, skills, capture, theme, phone, pr, coordination, team, denials, subagent, composer, naming, conversation, 'conversation-move': conversationMove, hovercard, mention, providers, bots, 'work-item': workItem, 'work-item-review': workItemReview, 'first-run': firstRun, creatures, bot, 'space-header': spaceHeader, sidebar, 'global-chats': globalChats, navigate };
if (!scenarios[scenario] || !base) {
  console.error('usage: node drive.mjs live|brief|attach|cap|pages|stop|wrap|teams|team-management|retry|rollback|inbox|inbox-groups|diff|memory|inspector|review|board|workshop|skills|capture|theme|phone|pr|coordination|team|denials|subagent|composer|naming|conversation|conversation-move|hovercard|mention|providers|bots|work-item|work-item-review|first-run|creatures|bot|space-header|sidebar|global-chats|navigate|pending-memory BASE_URL');
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
  // A Space of a Work item runs in a git worktree of the project, so the folder needs a commit.
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
// Opens the sidebar's + menu and picks `item`: New Work item, New Space or New conversation.
async function plus(item) {
  await aside.getByRole('button', { name: 'New', exact: true }).click();
  await aside.getByRole('menuitem', { name: item }).click();
}
// Adds a work item and returns its id, which names its worktree folder and branch.
async function addWorkItem(name) {
  await plus('New Work item');
  await aside.getByLabel('Work item title').fill(name);
  await aside.getByRole('button', { name: 'Add', exact: true }).click();
  await page.getByRole('heading', { name, level: 1 }).waitFor();
  return page.url().split('/').pop();
}
// Starts a claude Space from the form in `box` and returns its id. The sidebar (`aside`) opens it from the + menu.
async function newSpace(box, openName, name, team, coordinator) {
  if (box === aside) await plus(openName);
  else await box.getByRole('button', { name: openName }).click();
  if (team) await box.getByRole('button', { name: team, exact: true }).click();
  else await box.getByLabel(/^Include /).first().check();
  if (coordinator) await box.getByLabel(`${coordinator} coordinates`).check();
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
// The top-level message rows of the main list that `author` wrote (they have the reply-in-thread icon or the replies button).
const replyButtonName = /^Reply in thread$|^\d+ replies? in thread$/;
const rootRow = (author) =>
  main.locator('ul > li').filter({ has: page.getByRole('button', { name: replyButtonName }) }).filter({ has: page.getByText(author, { exact: true }) });
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
const sendJson = async (method, route, body) => {
  const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  return { status: res.status, body: await res.json().catch(() => null) };
};
// A Bot decides whether a Space may write, so a scenario that needs edits makes this one Bot and a Team that leads with it.
async function writerTeam() {
  let bot = (await getJson('/bots')).find((b) => b.name === 'writer');
  if (!bot) {
    bot = (await sendJson('POST', '/bots', { name: 'writer', description: 'Writes the files the user asks for.', backend: 'claude', write: true, permissionMode: 'acceptEdits', tools: ['Read', 'Grep', 'Glob', 'Edit', 'Write', 'Bash'] })).body;
  }
  await sendJson('PUT', '/teams/writers', { name: 'writers', responsibility: 'Edits files.', roles: [{ botId: bot.id, lead: true }] });
  return 'writers';
}

async function live() {
  writeFileSync(path.join(projectDir, 'notes.txt'), 'Reef window check: notes file.\n');
  writeFileSync(path.join(projectDir, 'todo.txt'), 'Reef window check: todo file.\n');
  const section = await addProject();
  await addWorkItem('Login');
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

async function teamManagement() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await page.goto(base + '/teams');
  await main.getByRole('heading', { name: 'Crews', level: 1 }).waitFor();

  const name = `delivery-${Date.now().toString(36)}`;
  await main.getByRole('button', { name: 'New crew' }).click();
  const dialog = page.getByRole('dialog', { name: 'New crew' });
  await dialog.getByLabel('Name', { exact: true }).fill(name);
  await pick('Start from', 'full', dialog);
  await dialog.getByLabel('Description').fill('Own desktop releases.');
  await dialog.getByRole('button', { name: 'Create crew' }).click();
  await page.waitForURL((url) => url.searchParams.get('team') === name);
  await main.getByLabel('Crew name').waitFor();

  let saved = await getJson(`/teams/${name}`);
  check(
    saved.roles.some((role) => role.name === 'lead' && role.lead) && saved.roles.some((role) => role.name === 'developer'),
    'the window creates a Team with a lead and a developer',
    saved.roles.map((role) => `${role.name}${role.lead ? '*' : ''}`).join(', '),
  );
  check(saved.responsibility === 'Own desktop releases.', 'the responsibility entered at creation round-trips', saved.responsibility);

  await main.getByLabel('Description').fill('Own desktop releases and rollback.');
  await main.getByRole('button', { name: 'Save crew' }).click();
  await main.getByText('Saved', { exact: true }).waitFor();
  saved = await getJson(`/teams/${name}`);
  check(saved.responsibility === 'Own desktop releases and rollback.', 'editing responsibility saves through REST', saved.responsibility);
  await shot('team-1280x800');

  await page.setViewportSize({ width: 390, height: 844 });
  const width = await page.evaluate(() => document.documentElement.scrollWidth);
  check(width <= 390, 'the Team editor has no horizontal scroll at 390x844', `${width} px`);
  await main.getByLabel('Description').scrollIntoViewIfNeeded();
  await shot('team-390x844');

  await page.setViewportSize({ width: 1280, height: 800 });
  await addWorkItem('Team run');
  const { id } = await newTeamSpace(name, 'delivery room');
  await send('Say ok in one word');
  await workingAs('lead');
  await idle();
  const turn = sql(`select role || '|' || status from turn where space_id = ${id} order by id limit 1`);
  check(turn === 'lead|done', 'the new Team runs a Space', turn);

  await page.goto(`${base}/teams?team=${encodeURIComponent(name)}`);
  await main.getByLabel('Crew name').waitFor();
  await main.getByRole('button', { name: 'Delete' }).click();
  const confirm = page.getByRole('dialog', { name: `Delete ${name}?` });
  await confirm.getByRole('button', { name: 'Delete crew' }).click();
  await page.waitForURL((url) => url.searchParams.get('team') === 'solo');
  check(sql(`select count(*) from team where name = '${name}'`) === '0', 'Delete team removes the Team while its Space keeps its Bots');
  check(Number(sql(`select count(*) from space_member where space_id = ${id}`)) >= 2, "the deleted Team's Space still has its Bots");
}

async function brief() {
  const section = await addProject();
  const itemId = await addWorkItem('Login');
  const res = await fetch(`${base}/work-items/${itemId}/brief`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ body: 'Goal: build the login page with magic links.', author: 'user' }),
  });
  check(res.status === 201, 'brief stored over REST', `PUT brief -> ${res.status}`);
  await page.reload();
  const cid = await newSpace(main, 'New Space', 'brief check');
  await send('what is the goal of this work item?');
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
  const checkpoints = main.getByRole('region', { name: 'Checkpoints' });
  const checkpointed = () => sql(`select id from turn where space_id=${cid} and checkpoint is not null order by id`);
  check(
    checkpointed() === '' && (await checkpoints.count()) === 0,
    'a free Space runs in a folder of its own, not a git repo, so its two turns have no checkpoint and the page lists none',
    `turns with a checkpoint: ${JSON.stringify(checkpointed())}`,
  );
  await addWorkItem('Search');
  await section.getByRole('link', { name: 'idea chat' }).click();
  await page.getByRole('heading', { name: 'idea chat', level: 1 }).waitFor();
  await shot('before-attach');
  await page.getByRole('button', { name: 'Attach to Work item' }).click();
  const dialog = page.getByRole('dialog', { name: 'Attach to a Work item' });
  await pick('Work item', 'Search', dialog);
  await shot('attach-form');
  await dialog.getByRole('button', { name: 'Attach', exact: true }).click();
  await page.getByText(/This Space is now attached to work item "Search"/).first().waitFor({ timeout: 15000 });
  await shot('attached');
  check(
    (await page.getByRole('button', { name: 'Attach to Work item' }).count()) === 0,
    'the Attach to Work item button is gone once the Space has a work item',
    `space row (work_item_id|kind): ${sql(`select work_item_id,kind from space where id=${cid}`)}`,
  );
  await section.getByRole('link', { name: '#idea chat' }).waitFor();
  check(
    sql(`select kind from space where id=${cid}`) === 'work_item',
    'the attached Space stays in the sidebar Spaces section',
    `space row: ${sql(`select id,work_item_id,kind from space where id=${cid}`)}`,
  );
  await exchange('Go on.');
  const lead = sql(`select count(*) from message where space_id=${cid} and author='lead'`);
  check(lead === '3', 'the Space still answers after the attach', `lead messages: ${lead}`);
  check(
    sql(`select count(*) from turn where space_id=${cid} and checkpoint is not null`) === '1',
    'the turn after the attach runs in the worktree and has a checkpoint',
    `turns with a checkpoint: ${JSON.stringify(checkpointed())}`,
  );
}

async function cap() {
  const section = await addProject();
  const itemId = await addWorkItem('Login');
  const cid = await newSpace(main, 'New Space', 'cap chat');
  await exchange('Describe a reef.');
  sql(`update work_item set token_budget=1 where id=${itemId}`, true);
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
  const state = sql(`select item.token_budget,space.paused from space join work_item item on item.id=space.work_item_id where space.id=${cid}`);
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
  await addWorkItem('Login');
}

async function capture() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  const sam = await (await fetch(base + '/bots', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: 'sam', backend: 'claude' }) })).json();

  await page.goto(base + '/capture');
  await pick('Capture project', name);
  await pick('Capture type', 'Bug');
  await page.getByLabel('Capture text').fill('Login fails on Safari');
  await page.getByLabel('Capture description').fill('Blank page after the redirect.');
  await shot('desktop-item');
  await page.setViewportSize({ width: 390, height: 844 });
  await shot('phone-item');
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.getByLabel('Capture text').press('Enter');
  await page.getByText('Captured').waitFor({ timeout: 15000 });
  await shot('captured');
  check((await page.getByLabel('Capture text').inputValue()) === '', 'the text box is empty after capture');
  const items = await getJson(`/work-items?project=${pid}`);
  const bug = items.find((i) => i.title === 'Login fails on Safari');
  check(
    items.length === 1 && bug?.type === 'bug' && bug.status === 'backlog' && bug.description === 'Blank page after the redirect.',
    'capture makes a Backlog bug with the typed title and description',
    JSON.stringify(items.map((i) => [i.type, i.status, i.title, i.description])),
  );

  // Talk to a Bot instead: the text is the conversation's first message and no Work item is made.
  await page.goto(base + '/capture');
  await pick('Capture project', name);
  await page.getByRole('switch', { name: 'Talk to a Bot' }).click();
  await pick('Capture Bot', 'sam');
  await page.getByLabel('Capture text').fill('Did I give you a codeword? Answer none.');
  await shot('desktop-talk');
  await page.setViewportSize({ width: 390, height: 844 });
  await shot('phone-talk');
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.getByLabel('Capture text').press('Enter');
  await page.getByText('Captured').waitFor({ timeout: 15000 });
  const spaces = (await getJson('/spaces')).filter((s) => s.projectId === Number(pid) && s.kind === 'conversation');
  const messages = spaces.length === 1 ? await getJson(`/messages?space=${spaces[0].id}`) : [];
  check(
    spaces.length === 1 && messages[0]?.body === 'Did I give you a codeword? Answer none.',
    'a conversation opens with the text as its first message',
    JSON.stringify(messages.map((m) => [m.author, m.body])),
  );
  const member = sql(`select bot_id from space_member where space_id=${spaces[0]?.id}`);
  check(member === String(sam.id), 'the conversation is with sam', member);
  check((await getJson(`/work-items?project=${pid}`)).length === 1, 'talking to a Bot makes no Work item');

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
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/bots.md)');
  const bodyBg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  const cdp = await page.context().newCDPSession(page);
  const system = (value) => cdp.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value }] });

  const section = await addProject();
  await addWorkItem('Login');
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
    await page.goto(`${base}/work-items/${fid}`);
    await page.getByRole('heading', { name: 'Login', level: 1 }).waitFor();
    await page.waitForTimeout(500);
    await shot(`work-item-${mode}`);
    await contrast('Work item page');
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
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/bots.md)');
  const section = await addProject();
  const itemId = await addWorkItem('Login');
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
  check(await row.isVisible(), 'the Spaces tab lists the Space', await main.innerText().then((t) => t.replace(/\s+/g, ' ').slice(0, 120)));
  await visit('needs-you', /^Needs you/, main.getByRole('link', { name: 'Open', exact: true }).first());
  const needsYou = (await getJson('/inbox')).filter((i) => i.group === 'needs_you').length;
  const badges = tabs.getByLabel(/waiting$/);
  const badge = needsYou > 0 ? await badges.textContent() : String(await badges.count());
  check(badge === String(needsYou), 'the Needs you tab badge counts only needs-you items, as REST does', `badge ${badge}, REST ${needsYou}`);
  await visit('board', /^Board/, main.getByRole('link', { name: /phone check/ }));
  await tabs.getByRole('button', { name: 'Menu' }).click();
  await aside.getByRole('link', { name: 'Crews', exact: true }).click();
  await page.getByRole('heading', { name: 'Crews', level: 1 }).waitFor();
  check(!(await aside.isVisible()), 'the sidebar sheet closes when a link in it is followed');
  await shot('teams');
  const teamsScroll = await overflow();
  check(teamsScroll.page <= 0 && teamsScroll.main <= 0, 'crews has no horizontal scroll at 390 px', `page ${teamsScroll.page}, main ${teamsScroll.main}`);

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

  await main.getByRole('button', { name: replyButtonName }).first().click();
  const thread = page.getByRole('complementary', { name: 'Thread' });
  await thread.waitFor();
  const tb = await box(thread);
  check(tb.top === 0 && tb.left === 0 && tb.right === 390 && tb.bottom === 844, 'a thread opens as its own full screen', JSON.stringify(tb));
  await shot('thread');
  await noScroll('thread');
  await thread.getByRole('button', { name: 'Close thread' }).click();
  await thread.waitFor({ state: 'hidden' });

  await main.getByRole('button', { name: 'Bots', exact: true }).click();
  const team = page.getByRole('complementary', { name: 'Bots' });
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
  await page.waitForURL('**/review');
  const files = main.getByRole('navigation', { name: 'Files' });
  await files.getByText('auth.go').waitFor({ timeout: 15000 });
  await files.waitFor();
  check(!(await tabs.isVisible()), 'the tab bar is hidden on the review page');
  let fb = await box(files);
  for (let i = 0; i < 50 && fb.right === fb.left; i++) {
    await page.waitForTimeout(100);
    fb = await box(files);
  }
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
  await aside.getByRole('link', { name: /^Inbox/ }).waitFor();
  check(!(await tabs.isVisible()), 'the tab bar goes away at 1024 px');
  check(await page.evaluate(() => window.__marker === 'kept'), 'widening keeps the same page, no reload');
  await shot('widened');
}

async function stop() {
  const section = await addProject();
  await addWorkItem('Login');
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
  await addWorkItem('Login');
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
  const itemId = await addWorkItem('Login');
  const cid = await newSpace(main, 'New Space', 'rollback chat', await writerTeam(), 'writer');
  const tree = path.join(data, 'worktrees', sql(`select project_id from Space where id=${cid}`), itemId);
  const state = () => [git(tree, 'rev-parse', 'HEAD'), git(tree, 'status', '--porcelain'), git(tree, 'log', '--oneline'), git(tree, 'ls-files')].join('\n--\n');

  await exchange('Describe a reef.');
  writeFileSync(path.join(tree, 'mine.txt'), 'my notes\n');
  const before = state();
  await send('Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.');
  await working();
  await idle();
  git(tree, 'add', '-A');
  git(tree, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'by the turn');
  const turnId = sql(`select id from turn where space_id=${cid} order by id desc limit 1`);
  const rollbackIcon = main.getByRole('button', { name: new RegExp(`^Roll back · Before turn ${turnId} · writer`) });
  await rollbackIcon.waitFor({ state: 'attached', timeout: 15000 });
  await shot('checkpoints');
  check(
    git(tree, 'log', '--oneline').includes('by the turn') && (await main.getByRole('button', { name: /^Roll back · Before turn/ }).count()) === 2,
    'each turn has a rollback icon on its reply and the turn left a commit and new.txt',
    `files: ${git(tree, 'ls-files')}; log:\n${git(tree, 'log', '--oneline')}`,
  );
  let asked = '';
  page.once('dialog', (d) => {
    asked = d.message();
    d.accept();
  });
  await rollbackIcon.locator('xpath=ancestor::li[1]').hover();
  await shot('checkpoint-hover');
  await rollbackIcon.click();
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
  const itemId = await addWorkItem('Login');
  const cid = await newSpace(main, 'New Space', 'diff chat');

  await page.goto(`${base}/work-items/${itemId}/review`);
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
  const json = await getJson(`/work-items/${itemId}/diff`);
  check(
    json.length === 1 && json[0].path === 'new.txt' && json[0].status === 'added' && json[0].hunks[0].lines[0].text === 'hi',
    'GET /work-items/{id}/diff returns the file with its added line',
    JSON.stringify(json),
  );

  await page.goto(`${base}/work-items/${itemId}`);
  const meta = main.getByRole('link', { name: /^1 file/ });
  await meta.waitFor();
  await shot('summary');
  check(
    (await meta.innerText()).replace(/\s+/g, ' ').includes('1 file +1 −0') &&
      (await meta.getAttribute('href')) === `/work-items/${itemId}/review`,
    'the Work item page header meta line sums the changes and links to the review page',
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
  const current = await getJson(`/work-items/${itemId}/diff?scope=current`);
  check(current.length === 1 && current[0].path === 'later.txt', 'GET /work-items/{id}/diff?scope=current returns only the uncommitted file', JSON.stringify(current));
  const chip = page.getByRole('button', { name: 'Diff +2 -0' });
  await chip.waitFor({ timeout: 15000 });
  const itemPage = await browser.newPage();
  await itemPage.goto(`${base}/work-items/${itemId}`);
  const summary = itemPage.getByRole('link', { name: /^2 files/ });
  await summary.waitFor();
  check((await summary.innerText()).replace(/\s+/g, ' ').includes('2 files +2 −0'), "the Space header's Diff chip and the Work item page sum the same branch diff", await summary.innerText());
  await itemPage.close();
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
  await page.goto(`${base}/work-items/${itemId}/review`);
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
  await page.waitForURL(`**/work-items/${itemId}/review`);
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
  await baseCard.getByText('Saved', { exact: true }).waitFor();
  check(sql(`select base_branch from project where id=${pid}`) === 'dev', 'saving the Base branch stores it', sql(`select id,base_branch from project where id=${pid}`));
  await page.reload();
  await baseField.waitFor();
  await page.waitForTimeout(500);
  await shot('base-branch');
  const baseValue = await baseField.inputValue();
  check(baseValue === 'dev', 'the saved base branch survives a reload', baseValue);

  const id = Date.now().toString(36);
  await main.getByRole('button', { name: 'New Work item' }).click();
  const itemDialog = page.getByRole('dialog', { name: 'New Work item' });
  await itemDialog.getByLabel('Work item title').fill(`Pp ${id}`);
  await itemDialog.getByRole('button', { name: 'Add', exact: true }).click();
  await main.getByRole('link', { name: `Pp ${id}` }).waitFor();
  await shot('work-item-added');
  check(
    sql(`select count(*) from work_item where project_id=${pid} and title='Pp ${id}'`) === '1',
    'New Work item on the project page stores the work item and lists it',
  );

  const session = `quick-${Date.now().toString(36)}`;
  await main.getByRole('button', { name: 'New free Space' }).click();
  const sessionDialog = page.getByRole('dialog', { name: 'New free Space' });
  await sessionDialog.getByLabel(/^Include /).first().check();
  await sessionDialog.getByLabel('Name', { exact: true }).fill(session);
  await sessionDialog.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('heading', { name: session, level: 1 }).waitFor();
  check(
    sql(`select count(*) from Space where id in (select space_id from space_project where project_id=${pid}) and work_item_id is null and name='${session}'`) === '1',
    'New free Space on the project page stores a Space with no Work item and opens it',
  );
  await page.goto(`${base}/projects/${pid}`);
  await main.getByRole('link', { name: session }).waitFor();
  await shot('quick-session-listed');

  const fid = sql(`select id from work_item where project_id=${pid} and title='Pp ${id}'`);
  const reefBin = process.env.REEF_UI_REEF;
  const reefOut = execFileSync(reefBin, ['memory', '--data', data, 'add', 'fact', 'Tokens live in Keychain.', '--scope', `work_item:${fid}`, '--as', 'developer'], { encoding: 'utf8' });
  console.log(reefOut.trim());
  await page.goto(`${base}/work-items/${fid}`);
  const itemMemory = main.getByRole('region', { name: 'Memory' });
  await itemMemory.getByText('Tokens live in Keychain.').waitFor();
  await page.waitForTimeout(500);
  await shot('work-item-memory');
  check(true, 'an item saved with reef memory add shows on the Work item page', await itemMemory.innerText());
  await itemMemory.getByRole('button', { name: /^Retire fact/ }).click();
  await itemMemory.getByText('Nothing remembered yet').waitFor();
  const itemRetired = sql(`select status||'|'||(retired_at is not null) from memory where scope_kind='work_item' and scope_id='${fid}'`);
  check(itemRetired === 'retired|1', 'Retire on the Work item page sets retired_at and the item leaves the list', itemRetired);
}

async function inspector() {
  const section = await addProject();
  await addWorkItem('Login');
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
  await addWorkItem('Subagent');
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
    throw new Error('apply the team full first (teams/full.json, see features/bots.md)');
  }
  ensureRepo();
  const section = await addProject();
  const itemId = await addWorkItem('Login');
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
  await page.goto(`${base}/work-items/${itemId}/review`);
  const files = main.getByRole('navigation', { name: 'Files' });
  const diffView = main.getByRole('region', { name: 'Diff' });
  const sendButton = main.getByRole('button', { name: /^Send to/ });
  await files.getByText('README.md').waitFor({ timeout: 15000 });
  const rows = (await files.getByRole('button').allInnerTexts()).map((r) => r.replace(/\s+/g, ' ').trim()).filter(Boolean).join(', ');
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
  if ((await fetch(`${base}/teams/duo`)).status !== 200) throw new Error('apply the team duo first (see features/bots.md)');
  // Headless Chromium denies notifications whatever is granted, so the window gets a stub
  // that is granted and records each one.
  await page.addInitScript(() => {
    window.__notes = [];
    window.Notification = class {
      static permission = 'granted';
      static requestPermission = async () => 'granted';
      constructor(title, options) {
        window.__notes.push({ title, body: options?.body, note: this });
      }
      close() {}
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
  await addWorkItem('Login');
  const { id: cid } = await newTeamSpace('duo', 'inbox room');

  await send('DESIGN a short plan for a login form.');
  await workingAs('lead', 60000);
  await aside.getByRole('link', { name: /^Inbox/ }).click();
  await page.getByRole('heading', { name: 'Inbox', level: 1 }).waitFor();
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
  const group = (await getJson('/inbox')).find((i) => i.kind === 'waiting')?.group;
  const badges = await aside.getByLabel(/waiting$/).count();
  check(group === 'fyi' && badges === 0, 'a waiting item is FYI, so the Inbox link shows no needs-you badge', `group ${group}, ${badges} badges`);
  await page.waitForTimeout(3000);
  const fired = await notes();
  check(fired.length === 1 && /^Waiting: inbox room$/.test(fired[0].title), 'one notification fired for the item', JSON.stringify(fired));

  await page.evaluate(() => window.__notes[0].note.onclick());
  await page.getByRole('heading', { name: 'inbox room', level: 1 }).waitFor();
  let landed = await landedOn('latest-message');
  check(page.url().endsWith(`/spaces/${cid}#latest-message`) && landed.inView && landed.ringed, 'clicking the notification opens the Space at the newest message, on screen and marked', `${page.url()} ${JSON.stringify(landed)}`);
  await aside.getByRole('link', { name: /^Inbox/ }).click();
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
  await page.getByRole('heading', { name: 'Inbox', level: 1 }).waitFor();
  await page.waitForTimeout(1000);
  check((await waiting.count()) === 0, 'the dismissed item stays gone after a reload');
  await sendApi(cid, 'Say ok in one word. Use no tools.');
  await waitWaiting(cid, dismissed.at);
  await waiting.waitFor({ timeout: 30000 });
  await shot('back-after-new-message');
  check(true, 'a new agent message brings the dismissed item back');
  await inboxGroups();
}

// The three groups of the inbox. Started and Review come from a queued item the lead submits;
// the other causes are rows written through SQLite, the way work-item seeds proof (their
// producers are proven by the server tests and the recipes).
async function pendingMemory() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = sql(`select id from project where name='${name}'`);
  const now = "strftime('%Y-%m-%dT%H:%M:%fZ','now')";
  const row = (kind, body, status) =>
    `insert into memory (kind, body, why, scope_kind, scope_id, author, role, bot_id, status, created_at, accepted_at) values ('${kind}', '${body}', '', 'project', '${pid}', 'lead', 'lead', 1, '${status}', ${now}, ${status === 'accepted' ? now : 'null'})`;
  sql(row('fact', 'Login uses sessions', 'accepted'), true);
  const oldId = sql(`select max(id) from memory`);
  sql(row('fact', 'Login uses magic links', 'candidate'), true);
  const newId = sql(`select max(id) from memory`);
  sql(`insert into memory_link (from_id, rel, to_kind, to_id) values (${newId}, 'replaces', 'memory', '${oldId}'), (${newId}, 'source', 'space', '1')`, true);
  sql(row('convention', 'Indent with tabs', 'candidate'), true);
  const convId = sql(`select max(id) from memory`);
  const status = (id) => sql(`select status||'|'||coalesce(replaced_by,'') from memory where id=${id}`);

  await page.goto(base + '/inbox');
  const list = main.getByRole('region', { name: /^Pending memory/ });
  await list.getByText('Login uses magic links').waitFor();
  await shot('pending');
  check((await list.getByRole('listitem').count()) === 2, 'the inbox lists both candidates under Pending memory');
  check((await list.getByText('Would retire memory ' + oldId).count()) === 1, 'the replacement shows the item it would retire');
  check((await list.getByText('Login uses sessions').count()) === 1, 'the retiring item shows its body beside the new one');
  check(status(oldId) === 'accepted|', 'before approval the old item is accepted');

  await list.getByRole('listitem').filter({ hasText: 'magic links' }).getByRole('button', { name: 'Approve' }).click();
  await list.getByText('Login uses magic links').waitFor({ state: 'hidden' });
  check(status(newId) === 'accepted|' && status(oldId) === `retired|${newId}`, 'Approve accepts the replacement and retires the old item', `${status(newId)} ${status(oldId)}`);
  await list.getByRole('listitem').filter({ hasText: 'Indent with tabs' }).getByRole('button', { name: 'Reject' }).click();
  await list.waitFor({ state: 'hidden' });
  check(status(convId) === 'rejected|', 'Reject turns the convention down', status(convId));
  await shot('empty');
}

async function inboxGroups() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = Number(sql(`select id from project where name='${name}'`));
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    const text = await res.text();
    return text ? JSON.parse(text) : null;
  };
  const eventually = async (ok, ms = 30000) => {
    const end = Date.now() + ms;
    while (!(await ok()) && Date.now() < end) await page.waitForTimeout(200);
    return ok();
  };
  const solo = (await getJson('/teams')).find((t) => t.name === 'solo');
  const item = await call('POST', '/work-items', { projectId: pid, title: 'Review me', teamId: solo.id, acceptanceLines: [{ text: 'Wrong password is refused' }, { text: 'Session lasts a day' }] });
  const idle = (sid) => sql(`select active_turn from space where id=${sid}`) === '';
  await call('PATCH', `/work-items/${item.id}`, { status: 'queued' });
  await eventually(async () => (await getJson(`/spaces?workItem=${item.id}`)).length > 0);
  const sid = (await getJson(`/spaces?workItem=${item.id}`))[0].id;
  await eventually(async () => idle(sid));
  await call('POST', `/spaces/${sid}/send`, { text: 'SUBMIT FOR REVIEW' });
  await eventually(async () => (await getJson(`/work-items/${item.id}`)).status === 'review');
  await eventually(async () => idle(sid));

  const now = "strftime('%Y-%m-%dT%H:%M:%fZ','now')";
  const capped = await call('POST', '/spaces', { projectId: pid, name: 'capped room' });
  sql(`update space set paused='token budget of 1000000 used: 1000312 tokens' where id=${capped.id}`, true);
  const held = await call('POST', '/spaces', { projectId: pid, name: 'held room' });
  sql(`insert into message (space_id, author, body, created_at) values (${held.id}, 'lead', '@architect plan it', ${now})`, true);
  sql(`insert into approval (space_id, from_role, to_role, message_id, text, status, kind, created_at) select ${held.id}, 'lead', 'architect', max(id), 'plan it', 'pending', 'hold', ${now} from message where space_id=${held.id}`, true);
  sql(`update space set paused='hop cap: 6 agent hops without a user message' where id=${held.id}`, true);
  const failedRoom = await call('POST', '/spaces', { projectId: pid, name: 'failed room' });
  sql(`insert into turn (space_id, role, backend, status, error, started_at, ended_at, wake_text) values (${failedRoom.id}, 'lead', 'claude', 'failed', 'server restarted', ${now}, ${now}, 'build it')`, true);
  const sub = await call('POST', '/work-items', { projectId: pid, parentId: item.id, title: 'Schema' });
  sql(`insert into inbox_fyi (kind, work_item_id, text, created_at) values ('sub_item', ${sub.id}, 'lead added sub-item ${sub.id} to work item ${item.id}: Schema', ${now})`, true);
  const proposed = await call('POST', '/work-items', { projectId: pid, type: 'bug', title: 'Slow deploy' });
  sql(`insert into inbox_fyi (kind, work_item_id, text, created_at) values ('proposed', ${proposed.id}, 'sam proposed work item ${proposed.id}: Slow deploy. It is in the Backlog until you queue it.', ${now})`, true);

  const groupOf = (title) => main.getByRole('region', { name: new RegExp(`^${title}`) });
  const rowOf = (group, text) => groupOf(group).getByRole('listitem').filter({ hasText: text });
  const items = await getJson('/inbox');
  const listed = (g) => items.filter((i) => i.group === g).map((i) => i.kind).sort().join(',');
  check(listed('needs_you') === 'failed,held,paused' && listed('review') === 'review' && listed('fyi').includes('proposed') && listed('fyi').includes('started') && listed('fyi').includes('sub_item'),
    'GET /inbox groups the seven causes: failed, held and paused in needs_you, review in review, started, sub_item and proposed in fyi', JSON.stringify(items.map((i) => `${i.group}/${i.kind}`)));
  for (const [width, height] of [[1280, 800], [390, 844]]) {
    await page.setViewportSize({ width, height });
    await page.goto(`${base}/inbox`);
    await page.getByRole('heading', { name: 'Inbox', level: 1 }).waitFor();
    await groupOf('Needs you').waitFor();
    await groupOf('Review').waitFor();
    await groupOf('FYI').waitFor();
    const needs = (await groupOf('Needs you').innerText()).replace(/\s+/g, ' ');
    const review = (await groupOf('Review').innerText()).replace(/\s+/g, ' ');
    const fyi = (await groupOf('FYI').innerText()).replace(/\s+/g, ' ');
    check(/Paused.*token budget of 1000000 used/i.test(needs) && /Held.*hop cap: 6 agent hops without a user message \(1 wake held\)/i.test(needs) && /Failed.*lead failed: server restarted/i.test(needs), `${width}x${height}: Needs you shows the paused, held and failed items`, needs.slice(0, 160));
    check(/Review.*Review me/.test(review), `${width}x${height}: Review shows the item in review`, review.slice(0, 160));
    check(/Started.*Started from the queue: Review me/i.test(fyi) && /Sub-item.*added sub-item \d+ to work item \d+: Schema/i.test(fyi) && /Proposed.*proposed work item \d+: Slow deploy/i.test(fyi), `${width}x${height}: FYI shows the started, sub-item and proposed items`, fyi.slice(0, 200));
    check((await groupOf('Needs you').getByRole('button', { name: 'Dismiss' }).count()) === 0 && (await groupOf('Review').getByRole('button', { name: 'Dismiss' }).count()) === 0 && (await groupOf('FYI').getByRole('button', { name: 'Dismiss' }).count()) >= 3,
      `${width}x${height}: only the FYI rows have Dismiss`);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    check(overflow <= 0, `${width}x${height}: the inbox has no horizontal scroll`, `${overflow} px`);
    await shot(`groups-${width}x${height}`);
    for (const g of ['Review', 'FYI']) {
      await groupOf(g).evaluate((el) => el.scrollIntoView({ block: 'start' }));
      await shot(`groups-${g.toLowerCase()}-${width}x${height}`);
    }
  }

  const paused = items.find((i) => i.kind === 'paused');
  const res = await fetch(`${base}/inbox/dismiss`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(paused) });
  const body = await res.text();
  check(res.status === 409 && /a paused item cannot be dismissed/.test(body), 'REST refuses to dismiss a Needs you item', `${res.status} ${body}`);

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${base}/inbox`);
  await rowOf('FYI', /Slow deploy/).getByRole('button', { name: 'Dismiss' }).click();
  await rowOf('FYI', /Slow deploy/).waitFor({ state: 'hidden' });
  await shot('groups-dismissed');
  await page.reload();
  await groupOf('FYI').waitFor();
  check((await rowOf('FYI', /Slow deploy/).count()) === 0 && sql("select count(*) from inbox_fyi where kind='proposed'") === '0' && (await getJson('/inbox')).filter((i) => i.group === 'needs_you').length === 3,
    'Dismiss on an FYI item removes its row for good, the Needs you items stay', sql('select kind from inbox_fyi'));
}

async function wrap() {
  const section = await addProject();
  await addWorkItem('Login');
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
  await page.goto(`${base}/work-items/${fid}`);
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
  await page.getByRole('heading', { name: 'Inbox', level: 1 }).waitFor();
  await shot('wrapped-needs-you');
  check(queued.length === 0, 'Needs you has no item for the wrapped Space', JSON.stringify(queued));
  await page.goto(`${base}/spaces/${cid}`);
  await main.getByText('Wrapped', { exact: true }).waitFor();
  await page.goto(`${base}/spaces`);
  await main.getByRole('link', { name: /#wrap chat/ }).waitFor();
  await shot('wrapped-spaces');
  check(
    (await section.getByRole('link', { name: /#wrap chat/ }).count()) === 0,
    'the wrapped Space is on the Spaces page and not in the sidebar',
  );
  await page.goto(`${base}/spaces/${cid}`);

  await main.getByRole('button', { name: 'Reopen' }).click();
  await page.getByLabel('Message').waitFor();
  check(sql(`select closed_at is null from Space where id=${cid}`) === '1', 'Reopen brings the composer back and clears closed_at');

  await page.goto(`${base}/work-items/${fid}`);
  await main.getByRole('button', { name: 'Drop' }).click();
  await page.getByRole('dialog', { name: 'Drop Work item' }).waitFor();
  await shot('drop-dialog');
  await page.getByRole('dialog', { name: 'Drop Work item' }).getByRole('button', { name: 'Drop Work item' }).click();
  await main.getByRole('button', { name: 'Drop' }).waitFor({ state: 'hidden' });
  await shot('dropped-work-item');
  check(
    sql(`select status from work_item where id=${fid}`) === 'dropped' &&
      sql(`select closed_at is not null from Space where id=${cid}`) === '1',
    'Drop closes the Work item and its Space',
  );
}

async function teams() {
  const missing = [];
  for (const name of ['duo', 'full']) if ((await fetch(`${base}/teams/${name}`)).status !== 200) missing.push(name);
  if (missing.length) throw new Error(`apply the teams first: missing ${missing.join(', ')} (see features/bots.md)`);
  const section = await addProject();
  await addWorkItem('Login');

  const { id: cid, options } = await newTeamSpace('duo', 'duo room', 'team-picked');
  check(options.includes('duo') && options.includes('full') && options.includes('solo'), 'the New Space form lists the teams', options.join(', '));
  const space = await getJson(`/spaces/${cid}`);
  const duoMembers = (await getJson(`/spaces/${cid}/members`)).map((m) => m.botId);
  const preview = (await getJson('/spaces')).find((s) => String(s.id) === cid);
  check(space.team === 'duo' && space.role === 'lead' && duoMembers.length === 2 && space.botId === duoMembers[0] && preview.members.join() === duoMembers.join(), 'the Space records its members, lead first, and team duo because a Crew was picked', JSON.stringify(preview));
  const bare = await sendJson('POST', '/spaces', { projectId: space.projectId, name: 'bare room', botIds: [duoMembers[1]] });
  const bareMembers = (await getJson(`/spaces/${bare.body.id}/members`)).map((m) => m.botId);
  check(bare.status === 201 && bare.body.team === '' && bare.body.botId === duoMembers[1] && bareMembers.join() === String(duoMembers[1]), 'POST /spaces with botIds and no team records the Bots, the first as coordinator, and no team', JSON.stringify(bare.body));

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
  const replyButton = rootRow('architect').last().getByRole('button', { name: 'Reply in thread' });
  check((await replyButton.count()) === 1, 'the architect root shows a Reply in thread button');
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
  const count = (await rootRow('architect').last().getByRole('button', { name: /replies in thread/ }).innerText()).trim();
  check(count === '2 replies in thread','the main list shows the root with a replies button', count);
  await shot('main-list');

  await addWorkItem('Handoff');
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

// The skill editor, a Bot's skill draft in its Space and in the library, and the picker.
async function skills() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const library = async () => getJson('/skills/library');
  await addProject();
  const lead = (await getJson('/bots')).find((b) => b.name === 'lead');
  await page.goto(`${base}/bots?bot=${lead.id}`);
  const lib = main.getByRole('region', { name: 'Skills library' });
  await lib.getByRole('button', { name: 'New skill' }).click();
  const form = lib.getByRole('form', { name: 'New skill' });
  await form.getByLabel('Skill name').fill('editor-skill');
  await form.getByLabel('SKILL.md').fill('---\nname: editor-skill\ndescription: Written in the editor.\n---\nSee notes/a.md.\n');
  await form.getByRole('button', { name: 'Add file' }).click();
  await form.getByLabel('Supporting file path').fill('notes/a.md');
  await form.getByLabel('Contents of notes/a.md').fill('EDITOR-NOTE-1');
  await shot('editor-new');
  await form.getByRole('button', { name: 'Create skill' }).click();
  await lib.getByRole('button', { name: 'Edit editor-skill' }).waitFor();
  let made = (await library()).find((s) => s.name === 'editor-skill');
  check(made?.version === 1 && made.sourceKind === 'reef', 'a skill written in the editor is saved at version 1', JSON.stringify(made));
  await lib.getByRole('button', { name: 'View files of editor-skill' }).click();
  await lib.locator('summary', { hasText: 'notes/a.md' }).click();
  check((await lib.getByText('EDITOR-NOTE-1').count()) === 1, 'the supporting file contents can be inspected');
  await lib.getByRole('button', { name: 'Edit editor-skill' }).click();
  await lib.getByLabel('Contents of notes/a.md').fill('EDITOR-NOTE-2');
  await shot('editor-edit');
  await lib.getByRole('button', { name: 'Save new version' }).click();
  await lib.getByRole('button', { name: 'Edit editor-skill' }).waitFor();
  made = (await library()).find((s) => s.name === 'editor-skill');
  const files = await getJson(`/skills/${made.id}`);
  check(made.version === 2 && files.files.some((f) => f.body === 'EDITOR-NOTE-2'), 'an edit saves version 2 with the new file', `v${made.version}`);
  check((await getJson(`/bots/${lead.id}`)).skills.length === 0, 'creating and editing a skill gives it to no Bot');

  const space = await (await fetch(`${base}/spaces`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: 'skill chat', botId: lead.id, kind: 'conversation' }) })).json();
  await page.goto(`${base}/spaces/${space.id}`);
  await send('DRAFT A SKILL');
  const card = page.getByRole('region', { name: 'Skill draft triage' });
  await card.waitFor({ timeout: 60000 });
  await card.getByText('SKILL.md').click();
  await shot('draft-in-space');
  check((await library()).every((s) => s.name !== 'triage'), 'a draft is not in the library while pending');
  await card.getByRole('button', { name: 'Save triage' }).click();
  await card.waitFor({ state: 'hidden' });
  check((await library()).some((s) => s.name === 'triage' && s.version === 1), 'saving the draft puts triage in the library at version 1');

  await send('DRAFT A SKILL');
  await card.waitFor({ timeout: 60000 });
  check((await card.getByRole('button', { name: 'Save as new version triage' }).count()) === 1, 'a draft named like a library skill offers Save as new version');
  await page.goto(`${base}/bots`);
  await lib.getByRole('heading', { name: 'Drafts' }).waitFor();
  await shot('drafts-in-library');
  await lib.getByRole('button', { name: 'Reject triage' }).click();
  await lib.getByRole('heading', { name: 'Drafts' }).waitFor({ state: 'hidden' });
  check((await library()).find((s) => s.name === 'triage').version === 1, 'rejecting a draft leaves the saved skill at version 1');
  check((await getJson(`/bots/${lead.id}`)).skills.length === 0, 'saving and rejecting drafts gives no Bot a skill');

  await page.goto(`${base}/bots?bot=${lead.id}`);
  await main.getByRole('combobox', { name: / add skill$/ }).click();
  await page.getByRole('listbox', { name: / add skill$/ }).getByRole('option', { name: /^triage · Reef/ }).click();
  await main.getByRole('button', { name: 'Save Bot' }).click();
  await page.getByText('Saved', { exact: true }).waitFor();
  await shot('picked');
  check(JSON.stringify((await getJson(`/bots/${lead.id}`)).skills) === '["reef:triage"]', 'the saved skill is picked for a Bot through the picker', JSON.stringify((await getJson(`/bots/${lead.id}`)).skills));

  const imported = path.join(projectDir, 'imported-skill');
  mkdirSync(path.join(imported, 'scripts'), { recursive: true });
  writeFileSync(path.join(imported, 'SKILL.md'), '---\nname: imported-skill\ndescription: Imported from a folder.\n---\n');
  writeFileSync(path.join(imported, 'scripts/run.sh'), 'echo run\n', { mode: 0o755 });
  const post = (route, body) => fetch(base + route, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const imp = await (await post('/skills/import', { path: imported })).json();
  const runSh = (await getJson(`/skills/${imp.id}`)).files.find((f) => f.path === 'scripts/run.sh');
  check(runSh && (await (await post(`/skills/${imp.id}/update`, {})).json()).version === 2, 'importing a folder keeps the execute bit and the REST update works', JSON.stringify(runSh));
  await page.goto(`${base}/bots?bot=${lead.id}`);
  await lib.getByRole('button', { name: 'Update imported-skill' }).click();
  await page.waitForFunction(async (id) => (await (await fetch(`/skills/library`)).json()).find((s) => s.id === id).version === 3, imp.id);
  check(true, 'the Update button in the window saves the next version (no 415)');

  await aside.getByRole('button', { name: 'Switch project' }).click();
  await page.getByRole('menuitem', { name: 'All projects', exact: true }).click();
  await aside.getByRole('region', { name: 'All projects' }).waitFor();
  await page.goto(`${base}/bots?bot=${lead.id}`);
  await main.getByRole('combobox', { name: / add skill$/ }).click();
  const offered = await page.getByRole('listbox', { name: / add skill$/ }).getByRole('option').allInnerTexts();
  await shot('picker-all-projects');
  check(offered.some((o) => o.startsWith('imported-skill · Reef')), 'under All projects the picker lists library skills', offered.join(' | '));
}
async function workshop() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) {
    throw new Error('apply the team full first (teams/full.json, see features/bots.md)');
  }
  sql(`insert or ignore into connection (name, command, args, env, url, created_at) values ('verify-conn', 'true', '[]', '{"API_KEY":"sk-verify-9911"}', '', '2026-01-01T00:00:00Z')`, true);
  const connBefore = `[${sql("select id from connection where name = 'verify-conn'")}]`;
  sql(`update bot set connections = '${connBefore}', tools = '["Read","Grep","Glob","Edit","Write","Bash"]', skills = '["claude:ghost-skill"]' where name = 'developer'`, true);
  mkdirSync(path.join(projectDir, '.claude/skills/workshop-skill'), { recursive: true });
  writeFileSync(path.join(projectDir, '.claude/skills/workshop-skill/SKILL.md'), '---\nname: workshop-skill\ndescription: Picked in the workshop scenario.\n---\n');

  const section = await addProject();
  const projectId = sql('select max(id) from project');
  await page.evaluate((id) => localStorage.setItem('reef.sidebar.projectId', id), projectId);
  const row = (col) => sql(`select ${col} from team_member m join bot on bot.id = m.bot_id where m.team_id = (select id from team where name = 'full') and bot.name = 'developer'`);

  // The developer's Bot panel: description, creature, skills and backend.
  const mate = row('bot.name');
  await page.goto(`${base}/bots?bot=${row('bot.id')}`);
  const personaBox = main.getByLabel(`${mate} description`);
  await personaBox.waitFor();
  const marker = `WORKSHOP-MARKER-${Date.now().toString(36)}`;
  const persona = `${await personaBox.inputValue()} Always say ${marker} once.`;
  await personaBox.fill(persona);
  await main.getByRole('button', { name: /^Change creature/ }).click();
  await main.getByRole('button', { name: 'Otter', exact: true }).click();
  await shot('persona-edited');
  const ghost = main.getByText('claude:ghost-skill').locator('..');
  check((await ghost.innerText()).includes('missing'), 'a picked skill that is not on disk shows as missing', await ghost.innerText());
  await main.getByRole('button', { name: 'Remove claude:ghost-skill', exact: true }).click();
  const addSkill = page.getByRole('listbox', { name: `${mate} add skill`, exact: true });
  await main.getByRole('combobox', { name: `${mate} add skill`, exact: true }).click();
  const offered = await addSkill.getByRole('option').allInnerTexts();
  check(offered.some((o) => o.startsWith('workshop-skill')), "the claude skill picker offers the project's skill", `${offered.length} offered`);
  await addSkill.getByRole('option', { name: /^workshop-skill/ }).click();
  check(!(await main.getByText('workshop-skill').locator('..').innerText()).includes('missing'), 'a picked skill that is on disk is not missing');
  await pick(`${mate} backend`, 'Codex', main);
  check((await main.getByLabel(`${mate} tool Read`).count()) === 0, 'the tool picker is hidden on a Codex Bot');
  await main.getByRole('combobox', { name: `${mate} add skill`, exact: true }).click();
  const codexOffered = await addSkill.getByRole('option').allInnerTexts();
  check(!codexOffered.some((o) => o.startsWith('workshop-skill')), 'a Codex Bot is not offered the Claude skills', `${codexOffered.length} offered`);
  await page.keyboard.press('Escape');
  await pick(`${mate} backend`, 'Claude Code', main);
  await pick(`${mate} permission mode`, 'acceptEdits', main);
  await main.getByRole('button', { name: 'Save Bot' }).click();
  await page.getByText('Saved', { exact: true }).waitFor();
  await shot('persona-saved');
  check(row('bot.description') === persona, "saving the developer's description on the Bots page updates the Bot row");
  check(row('bot.face') === 'otter', "picking a creature for the developer saves it to the Bot row");
  check(row('bot.skills') === '["claude:workshop-skill"]', "saving the developer's skills updates the Bot row", row('bot.skills'));
  check(row('bot.connections') === connBefore, "the developer's selected connections are unchanged by saving other settings");

  // Unrestricted Bash on a Bot without write: the server's error shows in the Bot panel.
  await page.goto(`${base}/bots?bot=${sql("select id from bot where name = 'architect'")}`);
  const bashBox = main.getByLabel('architect tool Bash');
  check(await bashBox.isDisabled(), 'the Bash checkbox of a read-only Bot is disabled', await bashBox.locator('..').getAttribute('title'));
  await main.getByLabel('architect add tool').fill('Bash(*)');
  await main.getByLabel('architect add tool').press('Enter');
  await main.getByRole('button', { name: 'Save Bot' }).click();
  const badTool = await errorText(/cannot have unrestricted Bash/);
  await shot('bad-tool-error');
  check(!!badTool, 'an invalid Bot (unrestricted Bash without write) shows the server error', badTool ?? '(not shown)');

  // Allowed tools are picked in the Bot panel and save; the MCP server stays.
  await page.goto(`${base}/bots?bot=${row('bot.id')}`);
  await main.getByLabel('developer tool Read').waitFor();
  check((await main.getByLabel('developer tool Read').isChecked()) && !(await main.getByLabel('developer tool WebFetch').isChecked()), 'the tool checkboxes show the Bot\'s tools');
  check(await main.getByLabel('developer tool mcp__verify-conn__*').count() === 1, 'the tool picker has one entry for the Bot\'s selected connection');
  await main.getByLabel('developer tool WebFetch').check();
  await main.getByLabel('developer add tool').fill('Bash(git log:*)');
  await main.getByLabel('developer add tool').press('Enter');
  await main.getByLabel('developer tool Write').uncheck();
  await shot('role-tools-mcp');
  await main.getByRole('button', { name: 'Save Bot' }).click();
  await page.getByText('Saved', { exact: true }).waitFor();
  check(row('bot.tools').includes('Bash(git log:*)') && row('bot.tools').includes('"WebFetch"') && !row('bot.tools').includes('"Write"'), "picking and unpicking allowed tools saves them to the Bot", row('bot.tools'));
  check(row('bot.connections') === connBefore, "the Bot's selected connection is unchanged by the panel", row('bot.connections'));

  // Duplicate makes a new Crew `reef team list` (the same store) shows.
  await page.goto(`${base}/teams?team=full`);
  const copyName = `workshop-copy-${Date.now().toString(36)}`;
  await main.getByRole('button', { name: 'Duplicate' }).click();
  const dup = page.getByRole('dialog', { name: 'New crew' });
  await dup.getByLabel('Name', { exact: true }).fill(copyName);
  await dup.getByRole('button', { name: 'Create crew' }).click();
  await main.getByRole('button', { name: copyName, exact: true }).waitFor({ timeout: 15000 });
  await shot('copied');
  check(
    sql(`select count(*) from team where name = '${copyName}'`) === '1',
    'Duplicate makes a new Crew',
  );

  // The next developer wake uses the new persona.
  await addWorkItem('Login');
  const { id: cid } = await newTeamSpace('full', 'workshop room');
  await send('@developer Say ok in one word.');
  await workingAs('developer');
  await idle();
  await shot('space-otter');
  const otters = await main.locator('svg [fill="#C08A5B"]').count();
  const penguins = await main.locator('svg [fill="#3FBFA0"]').count();
  check(otters > 0 && penguins === 0, "the Space shows the otter picked for the developer, not the penguin", `otter shapes ${otters}, penguin shapes ${penguins}`);
  const instructed = readFileSync(process.env.REEF_FAKE_LOG ?? '/dev/null', 'utf8').includes(marker);
  check(instructed, "the new Space's developer session starts with the new description in its system prompt", instructed ? marker : '(marker not in the fake claude log)');

  // A Bot edited after its session adopted: the Bots panel shows what the session runs with.
  const devId = row('bot.id');
  const devBot = await (await fetch(`${base}/bots/${devId}`)).json();
  const edited = await fetch(`${base}/bots/${devId}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...devBot, model: 'workshop-edited-model' }) });
  check(edited.status === 200, 'the developer Bot is edited after its session adopted', String(edited.status));
  await page.goto(`${base}/spaces/${cid}`);
  await main.getByRole('button', { name: 'Bots', exact: true }).click();
  const botsPanel = page.getByRole('complementary', { name: 'Bots' });
  await botsPanel.getByText('Refresh to adopt edits').waitFor();
  const adopted = await botsPanel.innerText();
  await shot('team-panel-adopted');
  check(adopted.includes('Session runs with') && !adopted.includes('workshop-edited-model'), "the Bots panel shows the adopted settings, not the Bot's edited model", adopted.replace(/\s+/g, ' ').slice(0, 200));
  check(adopted.includes('Bot settings changed. Refresh to adopt edits.'), 'the Bots panel says the Bot changed and Refresh adopts it');

  await page.goto(`${base}/settings?section=skills`);
  const skills = main.getByRole('region', { name: 'Skills' });
  await skills.getByRole('button', { name: 'Claude Code' }).click();
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
  const itemId = await addWorkItem('PR panel');
  const cid = await newSpace(main, 'New Space', 'pr chat');
  await fetch(`${base}/work-items/${itemId}/worktree`, { method: 'POST', headers: { 'Content-Type': 'application/json' } });

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
      text.includes('main ← work-item/') && text.includes('Approved') &&
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

  const shotFile = path.join(data, 'shot-pr.png');
  writeFileSync(shotFile, Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==', 'base64'));
  sql(`insert into proof (work_item_id, space_id, commands, screenshots, submitted_at) values (${itemId}, ${cid}, '[]', '["${shotFile}"]', '2026-10-27T10:00:00Z')`, true);
  sql(`update work_item set status='review' where id=${itemId}`, true);
  await page.goto(`${base}/work-items/${itemId}`);
  const proofCard = main.getByRole('region', { name: 'Proof' });
  const prLink = proofCard.getByRole('link', { name: '#12 Add the login page' });
  await prLink.waitFor({ timeout: 15000 });
  const proofImg = proofCard.getByRole('img', { name: 'Screenshot 1' });
  await proofImg.waitFor();
  await shot('review-proof');
  check(
    (await prLink.getAttribute('href')) === 'https://github.com/x/y/pull/12' && (await proofCard.innerText()).includes('Merged') &&
      (await proofImg.evaluate((el) => el.complete && el.naturalWidth > 0)),
    'the Proof card of an item in review shows the PR state and link and the screenshot as an image',
    (await proofCard.innerText()).replace(/\s+/g, ' '),
  );
}

// Bots' direct messages show as one compact item in the Space, never as bubbles. The
// lead's direct request runs the architect at once, and the fake architect answers slowly.
async function coordination() {
  for (const team of ['duo', 'full']) {
    if ((await fetch(`${base}/teams/${team}`)).status !== 200) throw new Error(`apply the team ${team} first (see features/bots.md)`);
  }
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await addWorkItem('Login');
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

  // The lead answers every ping and the architect pings again, until the hop cap holds the architect's wake for an answer.
  await page.setViewportSize({ width: 1280, height: 800 });
  const looped = page.getByRole('button', { name: /^architect and lead (are coordinating|coordinated) · \d+ messages?/i });
  await send('COORDINATE the endless review.');
  check(await until(looped, 'paused', 90000), 'an answered request whose response wake is held by the hop cap says paused', (await looped.allInnerTexts()).join(' | '));
  await looped.click();
  await panel.waitFor();
  check((await panel.getByRole('button', { name: 'Continue' }).count()) === 1, 'the panel offers Continue');
  await shot('desktop-4-paused');
  const pings = (await direct()).filter((m) => m.replyToId === null && m.from === 'architect');
  check(pings.length === 3 && pings.at(-1).state === 'answered', 'the held request is stored answered', pings.map((m) => m.state).join(','));
  await panel.getByRole('button', { name: 'Close coordination' }).click();

  // A user message drops the held wake: the request fails, and the item never reads completed.
  await send('Say ok in one word.');
  const seen = new Set();
  for (const end = Date.now() + 30000; Date.now() < end && !seen.has('failed'); await page.waitForTimeout(100)) seen.add(await stateOf(looped));
  check(seen.has('failed') && !seen.has('completed'), 'the dropped response wake reads failed and never completed', [...seen].join(','));
  check((await direct()).filter((m) => m.replyToId === null && m.from === 'architect').at(-1).state === 'failed', 'the dropped request is stored failed');
  await looped.click();
  await panel.waitFor();
  check((await panel.getByRole('button', { name: 'Retry' }).count()) === 0, 'it offers no Retry: no turn failed');
  await shot('desktop-4-dropped');

  // Two direct wakes are queued for the architect, one from each of two pairs: the lead's request and the
  // developer's, which the lead's turn caused by posting before it sent. Each panel shows only its own turn.
  await addWorkItem('Queued');
  const { id: qid } = await newTeamSpace('full', 'queued room');
  await send('QUEUE two reviews for the architect.');
  const pair = (from) => page.getByRole('button', { name: new RegExp(`^${from} and architect (are coordinating|coordinated) · \\d+ messages?`, 'i') });
  const architectRuns = async (id, timeout = 60000) => {
    for (const end = Date.now() + timeout; Date.now() < end; await page.waitForTimeout(100)) {
      const turn = (await getJson('/turns?status=running')).find((t) => t.spaceId === Number(qid) && t.role === 'architect');
      if (turn?.directMessageId === id) return true;
    }
    return false;
  };
  const panelHas = async (text, timeout = 0) => {
    for (const end = Date.now() + timeout; ; await page.waitForTimeout(100)) {
      if ((await panel.innerText()).includes(text)) return true;
      if (Date.now() >= end) return false;
    }
  };
  const requests = async () => (await getJson(`/spaces/${qid}/direct-messages`)).filter((m) => m.replyToId === null && m.to === 'architect');
  while ((await requests()).length < 2) await page.waitForTimeout(100);
  const [first, second] = await requests();
  check(first.from === 'lead' && second.from === 'developer', 'two requests to the architect are stored, from two pairs', `${first.from} ${first.id}, ${second.from} ${second.id}`);
  check(await architectRuns(first.id), "the architect runs the lead's request first, while the developer's is queued", `running ${first.id}`);
  await pair('lead').click();
  await panel.waitFor();
  check(await panelHas('Reviewing the plan', 8000), "the lead's panel streams the running turn");
  check(!(await panelHas('Checking the second plan')), "the lead's panel shows nothing of the queued turn");
  await shot('queued-first-lead');
  await pair('developer').click();
  check(!(await panelHas('Reviewing the plan')), "the developer's panel shows nothing of the running turn", (await panel.innerText()).replace(/\s+/g, ' '));
  check((await getJson('/turns?status=running')).some((t) => t.directMessageId === first.id), 'the first turn is still running when the panels are read');
  await shot('queued-first-developer');
  check(await architectRuns(second.id), "the architect then runs the developer's request", `running ${second.id}`);
  check(await panelHas('Checking the second plan', 8000), "the developer's panel streams its own turn");
  await shot('queued-second-developer');
  await pair('lead').click();
  check(!(await panelHas('Checking the second plan')), "the lead's panel still shows nothing of the other turn", (await panel.innerText()).replace(/\s+/g, ' '));
  check((await getJson('/turns?status=running')).some((t) => t.directMessageId === second.id), 'the second turn is still running when the panel is read');
  await shot('queued-second-lead');
}

async function team() {
  const section = await addProject();
  await addWorkItem('Login');
  await newTeamSpace('full', 'team room');
  await send('Say ok in one word.');
  await workingAs('lead');
  await idle();
  await send('@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.');
  const running = await workingAs('developer');
  check(running, 'the developer turn is running');
  const avatars = main.getByRole('button', { name: 'Show Bots' });
  check((await avatars.locator('.c-ring').count()) === 1, 'the header avatars ring one role while it works');
  check((await page.getByText('working…', { exact: true }).count()) === 0, 'no floating working badge');
  const panel = page.getByRole('complementary', { name: 'Bots' });
  check((await panel.count()) === 0, 'the team column is gone: nothing open until asked');
  const space = main.locator('section').first();
  const before = await space.boundingBox();
  await shot('closed-working');
  await main.getByRole('button', { name: 'Bots', exact: true }).click();
  await panel.waitFor();
  const pop = await panel.locator('xpath=../..').boundingBox();
  const head = await main.locator('header').boundingBox();
  const after = await space.boundingBox();
  check(pop.width === 320 && pop.y >= head.y + head.height && pop.x + pop.width <= 1200, 'the popover is 320 px wide under the header, inside the window', JSON.stringify(pop));
  check(after.width === before.width, 'opening it does not change the Space width', `${before.width} -> ${after.width}`);
  const rows = await panel.locator('ul').first().getByRole('listitem').allInnerTexts();
  check(rows.length === 4 && rows.some((r) => /developer[\s\S]*working/i.test(r)), 'it lists four roles and the developer as working', rows.join(' | ').replace(/\n/g, ' '));
  await shot('popover-working');
  await page.keyboard.press('Escape');
  await panel.waitFor({ state: 'hidden' });
  await avatars.click();
  await panel.waitFor();
  await page.mouse.click(300, 600);
  await panel.waitFor({ state: 'hidden' });
  check(true, 'Escape and a click outside both close the popover');
  await main.getByRole('button', { name: 'Bots', exact: true }).click();
  await panel.getByRole('button', { name: 'Close Bots' }).click();
  await panel.waitFor({ state: 'hidden' });
  check(true, 'its X closes it');
  await idle();
  check((await avatars.locator('.c-ring').count()) === 0, 'the ring goes when the turn ends');
  await shot('idle');
}

async function denials() {
  const roles = [
    { name: 'lead', lead: true, backend: 'claude', tools: ['Read'], description: 'You are the lead. Do what the user asks with the tools you have, then answer in one short sentence.' },
    { name: 'reviewer', backend: 'claude', tools: ['Read'], description: 'You are the reviewer. Do what the user asks with the tools you have, then answer in one short sentence. Never @mention anyone.' },
  ];
  const putTeam = async (r) => {
    const json = { 'Content-Type': 'application/json' };
    const bots = await getJson('/bots');
    const members = [];
    for (const { lead, ...profile } of r) {
      const have = bots.find((b) => b.name === profile.name);
      const saved = await (await fetch(base + (have ? `/bots/${have.id}` : '/bots'), { method: have ? 'PUT' : 'POST', headers: json, body: JSON.stringify(profile) })).json();
      members.push({ botId: saved.id, lead: !!lead });
    }
    return fetch(`${base}/teams/denials`, { method: 'PUT', headers: json, body: JSON.stringify({ name: 'denials', roles: members }) });
  };
  check((await putTeam(roles)).status === 200, 'the team denials is applied: a lead and a read-only reviewer, both allowed only Read');
  const toolsOf = async (role) => (await getJson(`/spaces/${cid}/members`)).find((r) => r.name === role).tools.join(',');
  const turnLines = (role) =>
    readFileSync(process.env.REEF_FAKE_LOG ?? '/dev/null', 'utf8').split('\n').filter((l) => l.includes(` role=${role} `));
  const allowed = (line) => /--allowedTools (\S+)/.exec(line)?.[1];
  const pidOf = (line) => /pid=(\d+)/.exec(line)?.[1];

  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  await addWorkItem('Fetch');
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

  const reviewerId = (await getJson('/teams/denials')).roles.find((r) => r.name === 'reviewer').botId;
  const { mcp, ...reviewer } = await getJson(`/bots/${reviewerId}`);
  const putReviewer = (backend) =>
    fetch(`${base}/bots/${reviewerId}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...reviewer, backend }) });
  check((await putReviewer('codex')).status === 200, 'the reviewer Bot is switched to codex');
  await page.reload();
  await card.getByText('Codex ignores allowed tools').waitFor();
  check((await add.count()) === 0, 'a codex role shows no Add');
  await page.waitForTimeout(500);
  await shot('codex-denied');
  await putReviewer('claude');

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
  await addWorkItem('Compose');
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
    const hint = await field.evaluate((el) => {
      const c = document.createElement('canvas').getContext('2d');
      c.font = getComputedStyle(el).font;
      return { text: el.placeholder, width: c.measureText(el.placeholder).width, room: el.clientWidth };
    });
    check(hint.width <= hint.room, `${label}: the placeholder fits the field without being cut`, `"${hint.text}" ${Math.round(hint.width)} px in ${hint.room} px`);
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
    await plus('New Space');
    await aside.getByLabel(/^Include /).first().check();
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
  check(/^\w+ \d.*\|1$/.test(placeholder), 'a Space started with an empty name has a placeholder name and waits for a proposal', placeholder);
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
  check(/^\w+ \d.*\|0$/.test(row(second)), 'a failed proposal keeps the placeholder name', row(second));
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
  const created = await (await post('POST', '/spaces', { projectId: project, name: 'claude 02:32 PM' })).json();
  await post('POST', `/spaces/${created.id}/send`, { text: probe });
  const renamed = await post('PUT', `/spaces/${created.id}`, { name: 'mine' });
  for (let i = 0; i < 150 && sql(`select count(*) from message where space_id=${created.id} and author<>'user'`) === '0'; i++) await page.waitForTimeout(200);
  await page.waitForTimeout(2000);
  check(renamed.status === 200 && row(created.id) === 'mine|0', 'a rename sent right after the first message is never overwritten', `${row(created.id)}; PUT ${renamed.status}`);
}

async function conversation() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const project = (await getJson('/projects')).at(-1).id;
  const call = (method, route, body) =>
    fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const sam = await (await call('POST', '/bots', { name: 'sam', backend: 'claude' })).json();
  const start = async () => {
    const was = page.url();
    await plus('New conversation');
    await page.getByRole('button', { name: 'sam', exact: true }).click();
    await page.waitForURL((u) => u.href !== was && /\/spaces\/\d+$/.test(u.pathname));
    return page.url().split('/').pop();
  };
  const named = async (id, name) => {
    for (let i = 0; i < 100 && sql(`select name_auto from Space where id=${id}`) !== '0'; i++) await page.waitForTimeout(200);
    await call('PUT', `/spaces/${id}`, { name });
    await section.getByRole('link', { name: new RegExp(name) }).waitFor();
  };
  const sidebarOrder = async () =>
    (await section.locator('a[href^="/spaces/"]').evaluateAll((as) => as.map((a) => a.getAttribute('href').split('/').pop())));
  const restOrder = async () => (await getJson(`/bots/${sam.id}/conversations`)).map((c) => String(c.id));
  // The sidebar refreshes its Spaces every 5 seconds.
  const sidebarIs = async (ids) => {
    for (let i = 0; i < 40 && (await sidebarOrder()).join() !== ids.join(); i++) await page.waitForTimeout(250);
    return (await sidebarOrder()).join() === ids.join();
  };

  const first = await start();
  await exchange('Remember the codeword plum-31 for this conversation only.');
  await named(first, 'codeword chat');
  const second = await start();
  await exchange('Did I give you a codeword? Answer with the codeword, or none if I did not.');
  await named(second, 'other chat');
  const kinds = sql(`select id, kind, team_id is null from Space where id in (select space_id from space_project where project_id=${project}) order by id`);
  check(kinds === `${first}|conversation|1\n${second}|conversation|1`, 'each start makes its own conversation with no Team', kinds);
  const sessions = sql(`select space_id, session_id from turn where space_id in (${first}, ${second}) order by id`).split('\n');
  check(sessions.length === 2 && sessions[0].split('|')[1] !== sessions[1].split('|')[1], 'the two conversations have their own session', sessions.join(' ; '));
  check(await sidebarIs([second, first]), 'the sidebar lists the newer conversation first', (await sidebarOrder()).join());
  check((await restOrder()).join() === [second, first].join(), 'GET /bots/{id}/conversations lists the newer conversation first', (await restOrder()).join());
  await shot('desktop-two');

  await section.getByRole('link', { name: /codeword chat/ }).click();
  await main.getByRole('heading', { name: 'codeword chat', level: 1 }).waitFor();
  await exchange('Which codeword did I just give you? Answer with the codeword only.');
  const own = sql(`select count(distinct session_id), count(*) from turn where space_id=${first}`);
  check(own === '1|2', 'continuing the older conversation resumes its one session', own);
  check(await sidebarIs([first, second]), 'the continued conversation moves to the top of the sidebar', (await sidebarOrder()).join());
  check((await restOrder()).join() === [first, second].join(), 'GET /bots/{id}/conversations puts it first too', (await restOrder()).join());
  await shot('desktop-continued');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('link', { name: 'Back' }).click();
  await page.getByRole('heading', { name: 'Spaces', level: 1 }).waitFor();
  await page.getByRole('navigation', { name: 'Tabs' }).getByRole('button', { name: 'Menu' }).click();
  const phoneRows = aside.getByRole('link', { name: /chat/ });
  await phoneRows.first().waitFor();
  check((await phoneRows.count()) === 2, 'the sidebar sheet lists both conversations at 390 px', String(await phoneRows.count()));
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  check(overflow <= 0, 'the phone list has no horizontal scroll', String(overflow));
  await shot('phone');
}

async function conversationMove() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const call = (method, route, body) =>
    fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  await call('POST', '/bots', { name: 'sam', backend: 'claude' });
  await call('POST', '/bots', { name: 'kit', backend: 'claude' });
  const start = async () => {
    const was = page.url();
    await plus('New conversation');
    await page.getByRole('button', { name: 'sam', exact: true }).click();
    await page.waitForURL((u) => u.href !== was && /\/spaces\/\d+$/.test(u.pathname));
    return page.url().split('/').pop();
  };
  const movedTo = async (id) => (await getJson(`/spaces/${id}`)).movedTo;
  const moved = async (id) => {
    for (let i = 0; i < 150 && (await movedTo(id)) === null; i++) await page.waitForTimeout(200);
    return movedTo(id);
  };
  const members = async (id) => (await getJson(`/spaces/${id}/members`)).map((m) => `${m.name}${m.lead ? '*' : ''}`).join();
  const kitAnswered = async (id) => {
    for (let i = 0; i < 150 && sql(`select count(*) from message where space_id=${id} and author='kit'`) === '0'; i++) await page.waitForTimeout(200);
    return sql(`select count(*) from message where space_id=${id} and author='kit'`) !== '0';
  };
  const login = 'Ada wants a login page with an email field and a Google button.';

  const typed = await start();
  await exchange(login);
  await send('@kit take over the login page. Use no tools and reply with one short sentence.');
  const typedTo = await moved(typed);
  check(typedTo !== null, 'a line starting @kit marks the conversation moved', `movedTo ${typedTo}`);
  await main.getByText('This conversation moved to a new Space').waitFor();
  const link = main.getByRole('link', { name: 'Open the Space' });
  check((await link.getAttribute('href')) === `/spaces/${typedTo}`, 'the conversation links to the new Space', await link.getAttribute('href'));
  check((await page.getByLabel('Message').count()) === 0, 'the moved conversation has no message box', String(await page.getByLabel('Message').count()));
  check((await main.getByRole('button', { name: 'Add Bot' }).count()) === 0, 'the moved conversation has no Add Bot button');
  check((await members(typedTo)) === 'sam*,kit', 'the new Space has sam leading and kit', await members(typedTo));
  const shape = sql(`select kind, team_id is null from Space where id=${typedTo}`);
  check(shape === 'free|1', 'the new Space is free and has no Team', shape);
  const kitPrompt = sql(`select prompt from turn where space_id=${typedTo} and role='kit' order by id limit 1`);
  const lines = sql(`select body from message where space_id=${typed} and id<(select max(id) from message where space_id=${typed})`).split('\n').filter(Boolean);
  check(kitPrompt.includes('sam wrote: @kit ') && !lines.some((l) => kitPrompt.includes(l)), "kit's first prompt has the summary and no message of the conversation", `${lines.length} conversation lines checked`);
  check(await kitAnswered(typedTo), 'kit answers in the new Space');
  await shot('desktop-moved');
  await link.click();
  await page.waitForURL(`**/spaces/${typedTo}`);
  await page.getByLabel('Message').waitFor();
  await shot('desktop-space');

  const clicked = await start();
  await exchange(login);
  await page.setViewportSize({ width: 390, height: 844 });
  await main.getByRole('button', { name: 'Add Bot' }).waitFor();
  const header = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  check(header <= 0, 'the conversation with the Add Bot button has no horizontal scroll at 390 px', String(header));
  await shot('phone-add-teammate');
  await page.setViewportSize({ width: 1280, height: 800 });
  await main.getByRole('button', { name: 'Add Bot' }).click();
  const dialog = page.getByRole('dialog', { name: 'Add a Bot' });
  await dialog.getByRole('button', { name: 'kit', exact: true }).waitFor();
  check((await dialog.getByRole('button', { name: 'sam', exact: true }).count()) === 0, "the dialog does not offer the conversation's own Bot");
  await shot('desktop-add-teammate');
  await dialog.getByRole('button', { name: 'kit', exact: true }).click();
  const clickedTo = await moved(clicked);
  check(clickedTo !== null, 'Add Bot marks the conversation moved', `movedTo ${clickedTo}`);
  check((await members(clickedTo)) === 'sam*,kit', 'the new Space has sam leading and kit', await members(clickedTo));
  await main.getByText('This conversation moved to a new Space').waitFor();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/spaces/${typed}`);
  await main.getByText('This conversation moved to a new Space').waitFor();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  check(overflow <= 0, 'the moved conversation has no horizontal scroll at 390 px', String(overflow));
  await shot('phone-moved');
}

async function hovercard() {
  if ((await fetch(`${base}/teams/full`)).status !== 200) throw new Error('apply the team full first (see features/window.md)');
  ensureRepo();
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const itemId = await addWorkItem('Hover');
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
    const files = await getJson(`/work-items/${itemId}/diff`);
    const lines = files.flatMap((f) => f.hunks.flatMap((h) => h.lines));
    const running = (await getJson('/turns?status=running')).filter((t) => String(t.spaceId) === cid).map((t) => t.role);
    const kinds = (await getJson('/inbox')).filter((i) => String(i.spaceId) === cid).map((i) => i.kind);
    state = kinds.includes('failed') ? 'a turn failed' : running.length ? 'working' : kinds.includes('waiting') ? 'waiting for you' : 'idle';
    const n = files.length;
    const changes = n ? `${n} file${n === 1 ? '' : 's'} +${lines.filter((l) => l.kind === 'add').length} −${lines.filter((l) => l.kind === 'del').length}` : 'no changes yet';
    return `#hover chat ${state} branch work-item/${itemId} changes ${changes} ${roles.map((r) => `${r} ${running.includes(r) ? 'working' : 'idle'}`).join(' ')}`;
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
    await fetch(`${base}/inbox/dismiss`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(i) });
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
  await addWorkItem('Mention');
  const { id: cid } = await newTeamSpace('full', 'mention chat');
  await send('@lead, @developer, @nobody, @tester: remember the secret word pineapple. Then write a 1500-word story about lighthouses. Use no tools.');
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
    return `${name} ${backend} · ${model} Edit ${role.description.split('\n')[0]} State ${running.includes(name) ? 'Working' : 'Idle'} Access ${role.write ? 'Can write' : 'Read-only'}${tools}`;
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
  await page.waitForURL(/\/bots\?/);
  const url = new URL(page.url());
  const devBot = (await getJson('/bots')).find((b) => b.name === 'developer');
  check(url.pathname === '/bots' && url.searchParams.get('bot') === String(devBot.id), 'Edit opens /bots with the developer Bot selected', page.url());
  const panel = page.getByRole('complementary', { name: 'Bot' }).or(page.locator('[aria-label=Bot]'));
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
    if ((await pane.count()) === 0) await main.getByRole('button', { name: replyButtonName }).first().click();
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
  const mate = `prov-${Date.now().toString(36)}`;
  const res = await fetch(`${base}/bots`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: mate, backend: 'claude', description: 'You check providers.' }) });
  check(res.status === 201, `the claude Bot ${mate} is made through POST /bots`, `${res.status}`);
  const mateId = (await res.json()).id;
  const roleOf = () => getJson(`/bots/${mateId}`);
  const openMate = async () => {
    await page.goto(`${base}/bots?bot=${mateId}`);
    await page.getByRole('combobox', { name: `${mate} provider`, exact: true }).waitFor();
  };
  const saveMate = async () => {
    await main.getByRole('button', { name: 'Save Bot' }).click();
    await page.getByText('Saved', { exact: true }).waitFor();
  };
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const setTheme = (t) => page.evaluate((t) => localStorage.setItem('reef.theme', t), t);

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

  await openMate();
  const select = page.getByRole('combobox', { name: `${mate} provider`, exact: true });
  check((await select.innerText()).trim() === 'Default (PATH)', 'a Bot with no provider shows Default (PATH)');
  await select.click();
  await page.getByRole('listbox', { name: `${mate} provider`, exact: true }).getByRole('option', { name, exact: true }).waitFor();
  const offered = await page.getByRole('listbox', { name: `${mate} provider`, exact: true }).getByRole('option').allInnerTexts();
  check(offered.join('|') === `Default (PATH)|${name}`, 'the claude Bot is offered PATH and the added claude install, not the detected one', offered.join('|'));
  await page.getByRole('listbox', { name: `${mate} provider`, exact: true }).getByRole('option', { name, exact: true }).click();
  await shot('desktop-dark-role');
  await saveMate();
  check((await roleOf()).provider === name, 'saving the Bot stores the provider (GET /bots/{id})', JSON.stringify((await roleOf()).provider));
  await openMate();
  check((await select.innerText()).trim() === name, 'after a reload the Bot shows its provider');

  await pick(`${mate} backend`, 'Codex', main);
  check((await select.innerText()).trim() === 'Default (PATH)', 'changing the backend to Codex resets a claude provider');
  await pick(`${mate} backend`, 'Claude Code', main);
  await saveMate();
  check(!(await roleOf()).provider, 'saving the reset Bot leaves no provider', JSON.stringify(await roleOf().then((r) => r.provider ?? null)));
  await pick(`${mate} provider`, name, main);
  await saveMate();

  for (const [label, width, height] of [['desktop', 1280, 800], ['phone', 390, 844]]) {
    await page.setViewportSize({ width, height });
    for (const mode of ['dark', 'light']) {
      await setTheme(mode);
      await page.goto(`${base}/settings?section=providers`);
      await added.waitFor();
      await page.waitForTimeout(400);
      check(await noOverflow(), `${label} ${mode}: Settings Providers has no horizontal scroll`);
      await shot(`${label}-${mode}-page`);
      await openMate();
      await page.waitForTimeout(400);
      check(await noOverflow(), `${label} ${mode}: the Bot panel has no horizontal scroll`);
      await shot(`${label}-${mode}-teammate-panel`);
    }
  }
  await setTheme('dark');

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${base}/settings?section=providers`);
  await added.getByRole('button', { name: `Remove ${name}` }).click();
  const inUse = await errorText(new RegExp(`used by bot ${mate}`), 15000);
  check(!!inUse && (await getJson('/providers')).some((p) => p.name === name), 'Remove of an install a Bot uses shows the server conflict and keeps it', inUse ?? '(not shown)');
  await shot('desktop-dark-in-use');
  await openMate();
  await pick(`${mate} provider`, 'Default (PATH)', main);
  await saveMate();
  await page.goto(`${base}/settings?section=providers`);
  await added.getByRole('button', { name: `Remove ${name}` }).click();
  await added.waitFor({ state: 'detached' });
  check((await getJson('/providers')).every((p) => p.name !== name), 'Remove deletes the install once no Bot uses it', JSON.stringify((await getJson('/providers')).map((p) => p.name)));
  await shot('desktop-dark-removed');
}

async function bots() {
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const suffix = Date.now().toString(36);
  const mate = `sam-${suffix}`;
  const teamName = `crew-${suffix}`;
  await page.setViewportSize({ width: 1280, height: 800 });
  await addProject();
  await page.goto(`${base}/bots`);
  await main.getByRole('heading', { name: 'Bots', level: 1 }).waitFor();

  await main.getByRole('button', { name: 'New Bot' }).click();
  const dialog = page.getByRole('dialog', { name: 'New Bot' });
  const description = `Checks the release notes of ${suffix}.`;
  const fields = await dialog.locator('label').allInnerTexts();
  check(
    fields.some((t) => t.startsWith('Name')) && fields.some((t) => t.startsWith('Describe this Bot')) && !fields.some((t) => /persona|routing|instruction/i.test(t)),
    'the New Bot dialog asks for Name and Describe this Bot, with no persona or routing field',
    fields.map((t) => t.split('\n')[0]).join(' | '),
  );
  await dialog.getByLabel('Name', { exact: true }).fill(mate);
  await dialog.getByLabel('Describe this Bot').fill(description);
  await shot('new-dialog');
  await dialog.getByRole('button', { name: 'Create Bot' }).click();
  await page.waitForURL((url) => url.searchParams.has('bot'));
  const id = Number(new URL(page.url()).searchParams.get('bot'));
  const panel = page.getByRole('complementary', { name: 'Bot' });
  await panel.waitFor();
  let bot = await getJson(`/bots/${id}`);
  check(
    bot.name === mate && bot.description === description && bot.backend === 'claude' && !bot.model && bot.tools.length === 0 && !bot.permissionMode && bot.write === false,
    'a Bot made from Name and a plain-language description alone is stored with default backend, no model, no tools, no permission mode and read-only',
    JSON.stringify({ name: bot.name, backend: bot.backend, model: bot.model, tools: bot.tools, permissionMode: bot.permissionMode, write: bot.write }),
  );
  check((await main.getByRole('region', { name: 'All bots' }).innerText()).includes(mate), 'the Bots page lists the new Bot');

  const after = async () => getJson(`/bots/${id}`);
  const save = async () => {
    await panel.getByRole('button', { name: 'Save Bot' }).click();
    await panel.getByText('Saved', { exact: true }).waitFor();
    await panel.getByText('Saved', { exact: true }).waitFor({ state: 'hidden', timeout: 30000 }).catch(() => {});
    return after();
  };
  await pick(`${mate} model`, 'Sonnet', panel);
  await shot('editor-1280x800');
  bot = await save();
  check(bot.model === 'sonnet' && bot.description === description && bot.write === false && !bot.permissionMode, 'saving only a model changes the model and nothing else', JSON.stringify({ model: bot.model, permissionMode: bot.permissionMode, write: bot.write }));
  await pick(`${mate} permission mode`, 'acceptEdits', panel);
  bot = await save();
  check(bot.permissionMode === 'acceptEdits' && bot.model === 'sonnet' && bot.write === false, 'saving only a permission mode changes the permission mode and nothing else', JSON.stringify({ model: bot.model, permissionMode: bot.permissionMode, write: bot.write }));
  await panel.getByRole('switch', { name: `${mate} tool Read` }).click();
  bot = await save();
  check(bot.tools.join() === 'Read' && bot.permissionMode === 'acceptEdits' && bot.model === 'sonnet', 'saving only a tool changes the tools and nothing else', JSON.stringify(bot.tools));
  await panel.getByLabel(`${mate} description`).fill(`${description} Edited.`);
  bot = await save();
  check(bot.description === `${description} Edited.` && bot.tools.join() === 'Read' && bot.model === 'sonnet', 'editing the description leaves model, tools and permission mode as they were', bot.description);

  await page.goto(`${base}/teams`);
  await main.getByRole('button', { name: 'New crew' }).click();
  const newTeam = page.getByRole('dialog', { name: 'New crew' });
  await newTeam.getByLabel('Name', { exact: true }).fill(teamName);
  await newTeam.getByRole('button', { name: 'Create crew' }).click();
  await page.waitForURL((url) => url.searchParams.get('team') === teamName);
  await main.getByLabel('Crew name').waitFor();
  await main.getByRole('button', { name: 'Add a Bot' }).click();
  const picker = page.getByRole('dialog', { name: 'Add a Bot' });
  await picker.getByRole('button', { name: new RegExp(`^${mate}`) }).click();
  const roles = main.getByRole('region', { name: 'Bots' });
  const leadSwitch = roles.getByRole('switch', { name: `${mate} suggested coordinator` });
  await leadSwitch.waitFor();
  const member = (await getJson(`/teams/${teamName}`)).roles.find((r) => r.botId === id);
  check(member && !member.lead && member.name === mate, 'adding a Bot from the picker puts it in the Team by Bot id', JSON.stringify(member && { botId: member.botId, name: member.name, lead: member.lead }));
  const editable = await roles.locator('textarea, input:not([role=switch]), [role=combobox]').count();
  check(
    editable === 0 && (await roles.getByRole('switch', { name: `${mate} write` }).count()) === 0,
    'the Team editor has no field, select or write switch for a member: only lead and Remove',
    `${editable} fields`,
  );
  await shot('team-editor-1280x800');
  await roles.getByRole('button', { name: new RegExp(`^${mate}`) }).click();
  await page.waitForURL((url) => url.pathname === '/bots' && url.searchParams.get('bot') === String(id));
  check(true, "a member row opens the Bot's own editor at /bots?bot=ID");
  const tools = (await after()).tools.join();
  check(tools === 'Read', "opening the editor from the Team changed nothing on the Bot", tools);

  for (const route of ['/bots', `/teams?team=${teamName}`, `/bots?bot=${id}`, `/bots/${id}`]) {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(base + route);
    await main.getByRole('heading', { level: 1 }).or(main.getByRole('region', { name: 'Bots' })).first().waitFor();
    check(await noOverflow(), `${route} has no horizontal scroll at 390x844`, `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
    await shot(`phone-${route.replace(/\W+/g, '-').replace(/^-|-$/g, '')}`);
  }
}

async function creatures() {
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  const suffix = Date.now().toString(36);
  const [teamA, teamB, teamC] = ['alpha', 'beta', 'gamma'].map((n) => `${n}-${suffix}`);
  const look = (t) => JSON.stringify({ face: t.face, colour: t.colour });
  await page.setViewportSize({ width: 1280, height: 800 });
  await addProject();

  const sam =await call('POST', '/bots', { name: `sam-${suffix}`, backend: 'claude', face: 'otter', colour: 'violet' });
  check(look(await getJson(`/bots/${sam.id}`)) === '{"face":"otter","colour":"violet"}', 'POST then GET /bots/{id} returns the creature and colour', look(sam));
  check(look((await getJson('/bots')).find((t) => t.id === sam.id)) === '{"face":"otter","colour":"violet"}', 'GET /bots lists them');
  const put = await call('PUT', `/bots/${sam.id}`, { name: sam.name, backend: 'claude', face: 'turtle', colour: 'teal' });
  check(look(put) === '{"face":"turtle","colour":"teal"}' && look(await getJson(`/bots/${sam.id}`)) === '{"face":"turtle","colour":"teal"}', 'PUT /bots/{id} changes them and GET returns the change', look(put));
  await call('PUT', `/bots/${sam.id}`, { name: sam.name, backend: 'claude', face: 'otter', colour: 'violet' });
  const bad = await fetch(base + '/bots', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: `bad-${suffix}`, backend: 'claude', colour: 'mauve' }) });
  check(bad.status === 400, 'an unknown colour is refused', `${bad.status}`);

  const ada = await call('POST', '/bots', { name: `ada-${suffix}`, backend: 'claude', face: 'whale' });
  const plain = await call('POST', '/bots', { name: `architect-${suffix}`, backend: 'claude' });
  const role = (bot, _name, extra) => ({ botId: bot.id, ...extra });
  await call('POST', '/teams', { name: teamA, roles: [role(sam, 'lead', { lead: true })] });
  await call('POST', '/teams', { name: teamB, roles: [role(ada, 'lead', { lead: true }), role(sam, 'developer')] });
  await call('POST', '/teams', { name: teamC, roles: [role(plain, 'architect', { lead: true })] });

  await page.goto(`${base}/bots`);
  await main.getByRole('heading', { name: 'Bots', level: 1 }).waitFor();
  const row = main.getByRole('region', { name: 'All bots' }).getByRole('button', { name: new RegExp(`^${sam.name}`) });
  await row.waitFor();
  const samFace = await row.locator('svg').first().innerHTML();
  check((await row.locator('svg').count()) === 1, 'Sam is one creature on the Bots page');
  check(await row.getByText(`${sam.name} · ${teamA}`, { exact: true }).isVisible(), `Sam's name in ${teamA} is a text badge`);
  check(await row.getByText(`${sam.name} · ${teamB}`, { exact: true }).isVisible(), `Sam's name in ${teamB} is a text badge`);
  check(samFace.includes('#8B5CF6') && !samFace.includes('#C08A5B'), "Sam's otter is drawn in violet, not the otter's own brown");
  await shot('bots-1280x800');

  const faceIn = async (team, roleName) => {
    await page.goto(`${base}/teams?team=${team}`);
    const button = main.getByRole('region', { name: 'Bots' }).getByRole('button', { name: new RegExp(`^${roleName}`) });
    await button.waitFor();
    return button.locator('svg').first().innerHTML();
  };
  const inA = await faceIn(teamA, sam.name);
  await shot('team-a-1280x800');
  const inB = await faceIn(teamB, sam.name);
  await shot('team-b-1280x800');
  check(inA === samFace && inB === samFace, 'Sam in two Teams is the same creature in both Teams and on the Bots page');
  check((await faceIn(teamB, ada.name)) !== samFace, "another Bot in that Team keeps its own creature");

  const pebble = await faceIn(teamC, plain.name);
  check(pebble.includes('#8E8E93') && !pebble.includes('#8B5CF6'), 'a Bot with no creature, named like a whale-faced role, is a pebble');

  await page.goto(`${base}/bots`);
  await main.getByRole('button', { name: 'New Bot' }).click();
  const dialog = page.getByRole('dialog', { name: 'New Bot' });
  const kit = `kit-${suffix}`;
  await dialog.getByLabel('Name', { exact: true }).fill(kit);
  await dialog.getByRole('button', { name: /^Change creature/ }).click();
  await dialog.getByRole('button', { name: 'Penguin', exact: true }).click();
  await dialog.getByRole('button', { name: /^Change creature/ }).click();
  await shot('picker-1280x800');
  await dialog.getByRole('button', { name: 'Pink colour', exact: true }).click();
  await dialog.getByRole('button', { name: 'Create Bot' }).click();
  await page.waitForURL((url) => url.searchParams.has('bot'));
  const made = (await getJson('/bots')).find((t) => t.name === kit);
  check(look(made) === '{"face":"penguin","colour":"pink"}', 'the New Bot dialog saves the creature and colour picked', look(made));

  await main.getByRole('button', { name: /^Change creature/ }).click();
  await main.getByRole('button', { name: 'Shark', exact: true }).click();
  await main.getByRole('button', { name: /^Change creature/ }).click();
  await main.getByRole('button', { name: 'Blue colour', exact: true }).click();
  await main.getByRole('button', { name: 'Save Bot' }).click();
  await main.getByText('Saved', { exact: true }).waitFor();
  check(look(await getJson(`/bots/${made.id}`)) === '{"face":"shark","colour":"blue"}', 'the Bot panel saves a new creature and colour', look(await getJson(`/bots/${made.id}`)));

  for (const route of ['/bots', `/teams?team=${teamB}`]) {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(base + route);
    await main.getByRole('heading', { level: 1 }).waitFor();
    check(await noOverflow(), `${route} has no horizontal scroll at 390x844`, `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
    await shot(`phone-${route.replace(/\W+/g, '-').replace(/^-|-$/g, '')}`);
  }
  await page.goto(`${base}/bots`);
  await main.getByRole('button', { name: 'New Bot' }).click();
  await page.getByRole('dialog', { name: 'New Bot' }).getByRole('button', { name: /^Change creature/ }).click();
  await shot('phone-picker');
}

async function sidebar() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const project = (await getJson('/projects')).at(-1).id;
  const call = (method, route, body) =>
    fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const sam = await (await call('POST', '/bots', { name: 'sidebar-sam', backend: 'claude' })).json();
  const urlPath = () => new URL(page.url()).pathname;
  const noScroll = () => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth);

  const search = aside.getByRole('searchbox', { name: 'Search' });
  const link = (name) => aside.getByRole('link', { name, exact: typeof name === 'string' });
  const y = async (loc) => (await loc.boundingBox()).y;
  const x = async (loc) => (await loc.boundingBox()).x;
  const topDown = [search, link(/^Inbox/), link(/^Board/), link('Spaces'), link('Conversations')];
  const ys = [];
  for (const loc of topDown) ys.push(await y(loc));
  const bottom = [link('Bots'), link('Crews'), link('Settings')];
  const bottomY = await y(bottom[0]);
  const bottomX = [];
  for (const loc of bottom) bottomX.push(await x(loc));
  check(
    ys.every((v, i) => i === 0 || v > ys[i - 1]) && ys[ys.length - 1] < bottomY && bottomX.every((v, i) => i === 0 || v > bottomX[i - 1]),
    'the sidebar lists Search, Inbox, Board, Spaces, Conversations top to bottom, then Bots, Crews, Settings at the bottom',
    `y: ${ys.join(',')}; bottom y ${bottomY}; bottom x: ${bottomX.join(',')}`,
  );
  check((await aside.getByRole('link', { name: /^Tasks/ }).count()) === 0 && (await aside.getByText('New channel').count()) === 0, 'no Tasks link and no New channel in the sidebar');
  await shot('desktop');

  const visits = [
    [link(/^Inbox/), '/inbox', 'Inbox'],
    [link(/^Board/), '/board', 'Board'],
    [link('Spaces'), '/spaces', 'Spaces'],
    [link('Conversations'), '/conversations', 'Conversations'],
    [link('Bots'), '/bots', 'Bots'],
    [link('Crews'), '/teams', 'Crews'],
    [link('Settings'), '/settings', 'Settings'],
  ];
  for (const [loc, route, heading] of visits) {
    await loc.click();
    await main.getByRole('heading', { name: heading, level: 1 }).waitFor();
    check(urlPath() === route, `the ${heading} link opens ${route}`, urlPath());
  }
  await aside.getByRole('link', { name: 'Today' }).click();
  await page.getByLabel('Quick capture').waitFor();
  check(urlPath() === '/', 'the logo opens Today');

  await plus('New Work item');
  await aside.getByLabel('Work item title').fill('Sidebar item');
  await aside.getByRole('button', { name: 'Add', exact: true }).click();
  await main.getByRole('heading', { name: 'Sidebar item', level: 1 }).waitFor();
  const item = sql(`select id,type,status from work_item where project_id=${project} and title='Sidebar item'`);
  check(urlPath() === `/work-items/${item.split('|')[0]}` && item.endsWith('|backlog'), '+ New Work item stores a Work item and opens it', `${item} at ${urlPath()}`);

  await plus('New Space');
  await aside.getByLabel('Name', { exact: true }).fill('free room');
  await aside.getByLabel(/^Include /).first().check();
  await aside.getByRole('button', { name: 'Start' }).click();
  await main.getByRole('heading', { name: 'free room', level: 1 }).waitFor();
  const free = sql(`select id,kind,work_item_id is null from space where id in (select space_id from space_project where project_id=${project}) and name='free room'`);
  check(urlPath() === `/spaces/${free.split('|')[0]}` && free.endsWith('|free|1'), '+ New Space stores a free Space and opens it', `${free} at ${urlPath()}`);
  await section.getByRole('link', { name: 'free room' }).waitFor();

  await plus('New conversation');
  await page.getByRole('button', { name: 'sidebar-sam', exact: true }).click();
  await page.waitForURL((u) => /\/spaces\/\d+$/.test(u.pathname) && !u.pathname.endsWith(`/${free.split('|')[0]}`));
  const conversation = sql(`select id,kind,team_id is null from space where id in (select space_id from space_project where project_id=${project}) and kind='conversation'`);
  check(urlPath() === `/spaces/${conversation.split('|')[0]}` && conversation.endsWith('|conversation|1'), '+ New conversation stores a conversation and opens it', `${conversation} at ${urlPath()}`);
  await call('PUT', `/spaces/${conversation.split('|')[0]}`, { name: 'sam chat' });
  const convoRow = section.getByRole('link', { name: /sam chat/ });
  await convoRow.waitFor({ timeout: 15000 });
  const fromTop = async (loc) => (await y(loc)) > (await y(link('Conversations')));
  check(await fromTop(convoRow), 'the conversation is listed under Conversations', await convoRow.innerText());
  await shot('created');

  await search.fill('zzz-no-match');
  await convoRow.waitFor({ state: 'hidden' });
  await search.fill('');
  await convoRow.waitFor();
  check(true, 'Search filters the Space and conversation rows');

  const own = Number(item.split('|')[0]);
  const room = await (await call('POST', '/spaces', { projectId: project, name: 'review room', workItemId: own })).json();
  const roomRow = section.getByRole('link', { name: '#review room' });
  await roomRow.waitFor({ timeout: 15000 });
  sql(`update work_item set status='in_progress', closed_at=null where id=${own}`, true);
  await page.waitForTimeout(6000);
  check((await roomRow.count()) === 1, 'the Space of an in-progress Work item is in the Spaces section');
  sql(`update work_item set status='review' where id=${own}`, true);
  await roomRow.waitFor({ state: 'hidden', timeout: 15000 });
  check(true, 'the Space leaves the Spaces section once its Work item is in Review', `item ${own} status ${sql(`select status from work_item where id=${own}`)}`);
  await page.goto(`${base}/spaces`);
  await main.getByRole('link', { name: '#review room' }).waitFor();
  check(true, 'the Spaces page still lists it', String(room.id));
  await shot('left-spaces');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/`);
  check(!(await aside.isVisible()), 'the sidebar is closed on a phone until Menu is tapped');
  const tabs = page.getByRole('navigation', { name: 'Tabs' });
  await tabs.getByRole('button', { name: 'Menu' }).click();
  await aside.waitFor();
  for (const [loc, , heading] of visits) check(await loc.isVisible(), `the phone sheet shows ${heading}`);
  check(await search.isVisible() && (await aside.getByRole('button', { name: 'New', exact: true }).isVisible()), 'the phone sheet shows Search and the + menu');
  const box = await aside.boundingBox();
  const sheet = await aside.evaluate((el) => ({ over: el.scrollWidth - el.clientWidth, fixed: getComputedStyle(el).position }));
  check(sheet.fixed === 'fixed' && box.x === 0 && box.y === 0 && box.height === 844 && box.width <= 390, 'the sidebar opens as a full-height sheet on the left', JSON.stringify(box));
  check(sheet.over <= 0 && (await noScroll()), 'the sheet has no horizontal scroll at 390 px', `sheet ${sheet.over}`);
  await shot('phone-sheet');
  await page.keyboard.press('Escape');
  await aside.waitFor({ state: 'hidden' });
  check(true, 'Escape closes the sheet');
  await tabs.getByRole('button', { name: 'Menu' }).click();
  await aside.getByRole('button', { name: 'New', exact: true }).click();
  await aside.getByRole('menuitem', { name: 'New Space' }).click();
  await aside.getByLabel(/^Include /).first().check();
  await aside.getByRole('button', { name: 'Start' }).click();
  await page.waitForURL((u) => /\/spaces\/\d+$/.test(u.pathname));
  await aside.waitFor({ state: 'hidden' });
  check(true, 'creating from the phone sheet opens the new Space and closes the sheet', urlPath());
}

async function bot() {
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  const reef = (...args) => execFileSync(process.env.REEF_UI_REEF, ['memory', '--data', data, ...args], { encoding: 'utf8' }).trim();
  const suffix = Date.now().toString(36);
  const [teamA, teamB] = ['alpha', 'beta'].map((n) => `${n}-${suffix}`);
  await page.setViewportSize({ width: 1280, height: 800 });
  await addProject();
  const project = (await getJson('/projects')).at(-1).id;

  const sam = await call('POST', '/bots', { name: `sam-${suffix}`, description: `Reviews the ${suffix} release notes.`, backend: 'claude', model: 'sonnet', face: 'otter', colour: 'violet' });
  const ada = await call('POST', '/bots', { name: `ada-${suffix}`, backend: 'claude' });
  const loner = await call('POST', '/bots', { name: `loner-${suffix}`, backend: 'claude' });
  const member = (t, _name, extra) => ({ botId: t.id, ...extra });
  await call('POST', '/teams', { name: teamA, roles: [member(ada, 'lead', { lead: true }), member(sam, 'developer')] });
  await call('POST', '/teams', { name: teamB, roles: [member(sam, 'reviewer', { lead: true })] });
  await call('PUT', `/bots/${sam.id}`, { name: sam.name, description: sam.description, backend: 'claude', model: 'sonnet', face: 'otter', colour: 'violet', write: true, permissionMode: 'acceptEdits' });

  const word = `note${suffix}`;
  const scope = `bot:${sam.id}`;
  for (const body of [`${word} always run the linter first`, `${word} prefers small commits`, `${word} retired habit`]) {
    reef('add', 'fact', body, '--scope', scope, '--as', 'user');
  }
  const retiredId = (await getJson(`/memory?scope=${scope}`)).find((m) => m.body.endsWith('retired habit')).id;
  reef('retire', String(retiredId));
  reef('add', 'fact', `${word} sits in the project scope`, '--scope', `project:${project}`, '--as', 'user');

  await call('POST', '/spaces', { projectId: project, name: 'older chat', botId: sam.id, kind: 'conversation' });
  await call('POST', '/spaces', { projectId: project, name: 'newer chat', botId: sam.id, kind: 'conversation' });

  await page.goto(`${base}/bots?bot=${sam.id}`);
  await main.getByRole('link', { name: 'Open profile' }).click();
  await page.waitForURL(`**/bots/${sam.id}`);
  await main.getByRole('heading', { name: sam.name, level: 1 }).waitFor();
  check(true, 'Open profile on the Bots page panel opens /bots/{id}');
  await main.getByRole('region', { name: 'Crews' }).getByRole('link').first().waitFor();

  const teamsCard = main.getByRole('region', { name: 'Crews' });
  const teamRows = (await teamsCard.getByRole('link').allInnerTexts()).map((t) => t.replace(/\s+/g, ' ').trim());
  const rest = (await getJson('/teams')).flatMap((t) => t.roles.filter((r) => r.botId === sam.id).map((r) => ({ team: t.name, role: r.name, write: r.write })));
  const botWrite = (await getJson(`/bots/${sam.id}`)).write;
  const shown = (r) => teamRows.some((t) => t.startsWith(`${r.team} ${r.role}`) && t.endsWith(r.write ? 'edits code' : 'read-only'));
  check(rest.length === 2 && rest.every(shown), 'both Teams show the Bot by its name with the Bot\'s one write access, and REST agrees', `${JSON.stringify(rest)} | ${JSON.stringify(teamRows)}`);
  check(botWrite === true && rest.every((r) => r.write === botWrite && r.role === sam.name), 'both Teams show the Bot\'s name and its own write access: a Team keeps none', JSON.stringify(rest));
  check((await main.getByText(`Reviews the ${suffix} release notes.`).count()) === 1, 'the description shows');
  check((await main.getByText('Sonnet', { exact: true }).or(main.getByText('sonnet', { exact: true })).count()) === 1, 'the model chip shows');

  const memoryCard = main.getByRole('region', { name: 'Memory' });
  await memoryCard.getByText(`${word} prefers small commits`).waitFor();
  const shownMemory = (await memoryCard.getByRole('listitem').allInnerTexts()).map((t) => t.replace(/\s+/g, ' ').trim()).sort();
  const recalled = reef('recall', word, '--scope', scope).split('\n').filter((l) => l.split('  ')[1] === 'accepted').map((l) => l.split('  ').slice(5).join('  ')).sort();
  const listed = (await getJson(`/memory?scope=${scope}`)).map((m) => `${m.kind} ${m.body}`).sort();
  check(
    recalled.length === 2 && JSON.stringify(shownMemory) === JSON.stringify(listed) && listed.every((l, i) => l.endsWith(recalled[i])),
    "the memory list equals recall over the Bot scope (retired and other-scope items left out)",
    `page ${JSON.stringify(shownMemory)} | recall ${JSON.stringify(recalled)}`,
  );
  const memoryCount = (await memoryCard.locator('h2 + span').innerText()).trim();
  check(memoryCount === '2', 'the memory count matches', memoryCount);

  const convoCard = main.getByRole('region', { name: 'Conversations' });
  await convoCard.getByRole('link', { name: /newer chat/ }).waitFor();
  const order = async () => (await convoCard.getByRole('link').evaluateAll((as) => as.map((a) => a.getAttribute('href').split('/').pop())));
  const restOrder = async () => (await getJson(`/bots/${sam.id}/conversations`)).map((c) => String(c.id));
  check((await order()).join() === (await restOrder()).join() && (await order()).length === 2, 'the conversations list matches REST, newest first', (await order()).join());
  check((await convoCard.getByRole('link').first().innerText()).includes('newer chat'), 'the newer conversation is first');
  await shot('desktop-1280x800');

  await main.getByRole('button', { name: 'New chat' }).click();
  await page.waitForURL((u) => /\/spaces\/\d+$/.test(u.pathname));
  const spaceId = page.url().split('/').pop();
  const row = sql(`select kind, team_id is null from Space where id=${spaceId}`);
  const members = sql(`select bot_id from space_member where space_id=${spaceId}`);
  check(row === 'conversation|1' && members === String(sam.id), 'New chat opens a new conversation with the Bot and no Team', `${row} members ${members}`);
  check(!(await restOrder()).slice(1).includes(spaceId) && (await restOrder())[0] === spaceId, 'REST lists it first', (await restOrder()).join());
  await page.goto(`${base}/bots/${sam.id}`);
  await convoCard.getByRole('link').nth(2).waitFor();
  check((await order()).join() === (await restOrder()).join() && (await order())[0] === spaceId, 'the profile lists it first after a reload', (await order()).join());

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${base}/bots/${sam.id}`);
  await main.getByRole('heading', { name: sam.name, level: 1 }).waitFor();
  await convoCard.getByRole('link').nth(2).waitFor();
  check(await noOverflow(), 'the profile has no horizontal scroll at 390x844', `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
  await shot('phone-390x844');
  await page.goto(`${base}/bots/${loner.id}`);
  await main.getByRole('heading', { name: loner.name, level: 1 }).waitFor();
  check((await main.getByText('In no Crew yet').count()) === 1 && (await main.getByText('No conversations yet').count()) === 1 && (await main.getByText('Nothing remembered yet').count()) === 1, 'a Bot with no Team, conversation or memory shows three empty states');
  await shot('phone-empty-390x844');
  await page.goto(`${base}/bots/999999`);
  await main.getByText('No Bot 999999').waitFor();
  check(true, 'an unknown Bot shows an error, not a blank page');
}

async function board() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = Number(sql(`select id from project where name='${name}'`));
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  const suffix = Date.now().toString(36);
  const teamName = `crew-${suffix}`;
  const mate = await call('POST', '/bots', { name: `lead-${suffix}`, backend: 'claude' });
  const crew = (await call('POST', '/teams', { name: teamName, roles: [{ botId: mate.id, lead: true }] })).id;
  const make = async (title, { status, ...extra } = {}) => {
    const w = await call('POST', '/work-items', { projectId: pid, title, ...extra });
    if (status) sql(`update work_item set status='${status}' where id=${w.id}`, true);
    return w;
  };
  const items = async () => (await getJson('/work-items')).filter((i) => i.projectId === pid);
  const byTitle = async (title) => (await items()).find((i) => i.title === title);
  const eventually = async (ok) => {
    const end = Date.now() + 10000;
    while (!(await ok()) && Date.now() < end) await page.waitForTimeout(200);
    return ok();
  };

  // The Team is busy (an item needs the user), so queued items stay queued. Items with no Team
  // are not started by the queue, so nothing runs a turn. Statuses a client cannot set on create
  // are written to the database.
  await make('busy item', { teamId: crew, status: 'needs_you' });
  await make('draft idea', { teamId: crew });
  await make('login bug', { type: 'bug', teamId: crew });
  for (const t of ['queue one', 'queue two', 'queue three']) {
    const w = await make(t, { teamId: crew });
    await call('PATCH', `/work-items/${w.id}`, { status: 'queued' });
  }
  await make('running job', { type: 'task', status: 'in_progress' });
  await make('review me', { status: 'review' });
  await make('finished job', { status: 'done' });
  await make('dropped idea', { status: 'dropped' });

  const region = (label) => main.getByRole('region', { name: label, exact: true });
  const card = (title) => main.getByRole('link', { name: title });
  const titles = (label) => region(label).getByRole('link').allInnerTexts().then((ts) => ts.map((t) => t.split('\n').filter((l) => !/^\d+$/.test(l))[0]));
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(base + '/board');
  await card('busy item').waitFor();
  await shot('1280x800');
  const headings = await Promise.all(['Backlog', 'Queued', 'In progress', 'Needs you', 'Review', 'Done', 'Dropped'].map(async (l) => [l, await region(l).getByRole('heading', { level: 2 }).boundingBox()]));
  check(headings.every(([, b]) => b && b.x >= 0 && b.x + b.width <= 1280), 'all seven column headings are inside the 1280 px window', headings.map(([l, b]) => `${l} ${Math.round(b.x)}-${Math.round(b.x + b.width)}`).join(', '));
  check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth) && (await region('Dropped').evaluate((el) => el.parentElement.scrollWidth <= el.parentElement.clientWidth)), 'the board at 1280 px needs no horizontal scroll');
  await page.evaluate(() => localStorage.setItem('reef.theme', 'light'));
  await page.reload();
  await card('busy item').waitFor();
  const lit = await card('busy item').evaluate((el) => [getComputedStyle(el).backgroundColor, getComputedStyle(el.closest('section')).backgroundColor]);
  await shot('light-1280x800');
  check(lit[0] !== lit[1], "in the light theme a card's background differs from its column's", `card ${lit[0]}, column ${lit[1]}`);
  await page.evaluate(() => localStorage.removeItem('reef.theme'));
  await page.reload();
  await card('busy item').waitFor();

  const columns = {
    Backlog: ['draft idea', 'login bug'],
    Queued: ['queue one', 'queue two', 'queue three'],
    'In progress': ['running job'],
    'Needs you': ['busy item'],
    Review: ['review me'],
    Done: ['finished job'],
    Dropped: ['dropped idea'],
  };
  for (const [label, want] of Object.entries(columns)) {
    const got = await titles(label);
    check(JSON.stringify(got) === JSON.stringify(want), `${label} lists its items`, got.join(', '));
  }

  await pick('Type', 'bug');
  await card('draft idea').waitFor({ state: 'hidden' });
  await region('Backlog').getByRole('link', { name: 'login bug' }).waitFor();
  await eventually(async () => (await main.getByRole('link').count()) === 1);
  const shown = await main.getByRole('link').allInnerTexts();
  check(shown.length === 1 && shown[0].startsWith('login bug'), 'the type filter hides the other types', shown.join(' | '));
  check(page.url().includes('type=bug'), 'the filter is in the URL', page.url());
  await shot('filter-bug');
  await pick('Type', 'All types');
  await pick('Crew', teamName);
  await card('running job').waitFor({ state: 'hidden' });
  check((await card('running job').count()) === 0 && (await card('busy item').count()) === 1, 'the Crew filter hides items of other Crews', (await main.getByRole('link').allInnerTexts()).length + ' cards');
  await pick('Crew', 'All Crews');
  await region('In progress').getByRole('link', { name: 'running job' }).waitFor();

  await card('draft idea').dragTo(region('Queued'));
  const drafted = () => byTitle('draft idea');
  check(await eventually(async () => (await drafted()).status === 'queued'), 'dragging a Backlog item into Queued queues it', JSON.stringify(await drafted().then((w) => [w.status, w.queuePosition])));
  check((await drafted()).queuePosition === 4, 'it joins the end of its Team queue', `position ${(await drafted()).queuePosition}`);
  await region('Queued').getByRole('link', { name: 'draft idea' }).waitFor();
  await card('draft idea').dragTo(region('Backlog'));
  check(await eventually(async () => (await drafted()).status === 'backlog'), 'dragging it back to Backlog takes it out of the queue', (await drafted()).status);
  await region('Backlog').getByRole('link', { name: 'draft idea' }).waitFor();

  const positions = async () => Object.fromEntries((await items()).filter((i) => i.status === 'queued').map((i) => [i.title, i.queuePosition]));
  await card('queue three').dragTo(card('queue one'));
  const ordered = async () => Object.entries(await positions()).sort((a, b) => a[1] - b[1]).map(([t, p]) => `${p}:${t}`).join(', ');
  check(await eventually(async () => (await ordered()) === '1:queue three, 2:queue one, 3:queue two'), 'dropping a queued item on another changes its queue position (REST)', await ordered());
  await page.waitForFunction(() => [...document.querySelectorAll('section[aria-label="Queued"] li a')].map((a) => a.textContent).join('|').startsWith('1queue three'));
  const queued = await titles('Queued');
  check(JSON.stringify(queued) === JSON.stringify(['queue three', 'queue one', 'queue two']), 'the Queued column shows the new order', queued.join(', '));
  await shot('reordered');

  // Chrome drops a drag that a scroll interrupts, so a wide window and the board scrolled to the
  // source column keep the source and the target in view together.
  await page.setViewportSize({ width: 1600, height: 900 });
  for (const [title, scroll, target] of [['running job', 'In progress', 'Review'], ['busy item', 'In progress', 'Done'], ['login bug', 'Backlog', 'In progress'], ['queue two', 'Queued', 'In progress'], ['review me', 'Queued', 'Queued']]) {
    await page.goto(base + '/board');
    await card(title).waitFor();
    await region(scroll).evaluate((el) => {
      const board = el.parentElement;
      board.scrollLeft += el.getBoundingClientRect().left - board.getBoundingClientRect().left;
    });
    const before = (await byTitle(title)).status;
    await card(title).dragTo(region(target));
    const alert = main.getByRole('alert');
    await alert.waitFor();
    const after = (await byTitle(title)).status;
    check(after === before && /through its own actions/.test(await alert.innerText()), `dragging "${title}" into ${target} is refused in the window`, `status ${before} -> ${after}; ${await alert.innerText()}`);
    if (target === 'Review' || target === 'Done') await shot(`refused-${target.toLowerCase()}`);
  }

  await make('no team item');
  await page.goto(base + '/board');
  await card('no team item').waitFor();
  await region('Backlog').evaluate((el) => {
    const board = el.parentElement;
    board.scrollLeft += el.getBoundingClientRect().left - board.getBoundingClientRect().left;
  });
  await card('no team item').dragTo(region('Queued'));
  const reason = main.getByRole('alert');
  await reason.waitFor();
  check((await byTitle('no team item')).status === 'backlog' && (await reason.innerText()) === 'Set a Crew on this item before queuing it.', 'dragging an item with no Crew into Queued is refused with the reason', `${(await byTitle('no team item')).status}; ${await reason.innerText()}`);
  await shot('refused-no-team');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(base + '/board');
  await card('busy item').waitFor();
  await shot('390x844');
  const o = await page.evaluate(() => {
    const m = document.querySelector('main');
    return { page: document.documentElement.scrollWidth - innerWidth, main: m.scrollWidth - m.clientWidth };
  });
  check(o.page <= 0 && o.main <= 0, 'the board has no horizontal scroll at 390 px', `page ${o.page}, main ${o.main}`);
  check(await page.getByRole('navigation', { name: 'Tabs' }).getByRole('link', { name: 'Board' }).isVisible(), 'the phone tab bar has a Board tab');
}

async function workItem() {
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  await page.setViewportSize({ width: 1280, height: 800 });
  const section = await addProject();
  const id = await addWorkItem('Work item page');
  const pid = sql(`select project_id from work_item where id=${id}`);
  const item = () => getJson(`/work-items/${id}`);
  const headerText = async () =>
    (await main.getByRole('heading', { name: 'Work item page', level: 1 }).locator('xpath=..').innerText()).replace(/\s+/g, ' ');
  const chips = await headerText();
  check(/feature/.test(chips) && /backlog/.test(chips), 'the page header shows the type and status', chips);

  const lines = main.getByRole('region', { name: 'Acceptance lines' });
  for (const text of ['Users can sign in', 'Wrong password shows an error']) {
    await lines.getByLabel('New acceptance line').fill(text);
    await lines.getByRole('button', { name: 'Add', exact: true }).click();
    await lines.getByText(text, { exact: true }).waitFor();
  }
  let got = (await item()).acceptanceLines;
  check(
    got.length === 2 && got[0].text === 'Users can sign in' && got[1].position === 2 && got[1].checked === false,
    'two acceptance lines added from the page match REST',
    JSON.stringify(got),
  );
  await lines.getByRole('button', { name: 'Remove line 1' }).click();
  await lines.getByText('Users can sign in', { exact: true }).waitFor({ state: 'hidden' });
  got = (await item()).acceptanceLines;
  check(got.length === 1 && got[0].position === 1 && got[0].text === 'Wrong password shows an error', 'Remove drops the line in REST and renumbers', JSON.stringify(got));

  const subs = main.getByRole('region', { name: 'Sub-items' });
  await subs.getByLabel('New sub-item').fill('Login form');
  await subs.getByRole('button', { name: 'Add', exact: true }).click();
  await subs.getByRole('link', { name: /Login form/ }).waitFor();
  const kids = await getJson(`/work-items?parent=${id}`);
  check(
    kids.length === 1 && kids[0].title === 'Login form' && kids[0].parentId === Number(id) && kids[0].status === 'backlog' && kids[0].projectId === Number(pid),
    'a sub-item added from the page matches REST',
    JSON.stringify(kids.map((k) => ({ id: k.id, title: k.title, parentId: k.parentId, status: k.status }))),
  );
  const subsText = (await subs.innerText()).replace(/\s+/g, ' ');
  check(subsText.includes('backlog'), 'the sub-item row shows its status', subsText);

  const solo = (await getJson('/teams')).find((t) => t.name === 'solo');
  const put = await fetch(`${base}/work-items/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...(await item()), teamId: solo.id }),
  });
  await fetch(`${base}/work-items/${id}/brief`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ body: 'Goal: email login for the shop.', author: 'user' }),
  });
  await page.reload();
  const header = await headerText();
  check(put.status === 200 && header.includes('solo'), 'the header shows the Team of the item', header);
  await main.getByText('Version 1 by').waitFor();
  check((await main.getByRole('region', { name: 'Brief' }).count()) === 1, 'a top-level item shows its brief');

  const proofCard = main.getByRole('region', { name: 'Proof' });
  await proofCard.getByText('Not submitted yet').waitFor();
  check(true, 'the proof section says Not submitted yet before a submit');
  const space = await (
    await fetch(`${base}/spaces`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ projectId: Number(pid), workItemId: Number(id), name: 'proof space' }),
    })
  ).json();
  const commands = JSON.stringify([{ command: 'make test', output: 'ok: 12 passed' }]);
  sql(`insert into proof (work_item_id, space_id, commands, screenshots, submitted_at) values (${id}, ${space.id}, '${commands}', '["/data/shot-login.png"]', '2026-10-27T10:00:00Z')`, true);
  sql(`update acceptance_line set checked=1, note='verified by the test' where work_item_id=${id} and position=1`, true);
  await page.reload();
  await proofCard.getByText('make test').waitFor();
  const proofText = (await proofCard.innerText()).replace(/\s+/g, ' ');
  const proof = await getJson(`/work-items/${id}/proof`);
  check(
    proofText.includes('$ make test') && proofText.includes('ok: 12 passed') && (await proofCard.getByRole('img', { name: 'Screenshot 1' }).count()) === 1 && proofText.includes(`Space ${space.id}`) && proof.commands[0].command === 'make test',
    'the proof section shows the submitted commands, output and screenshots read-only',
    proofText,
  );
  const linesText = (await lines.innerText()).replace(/\s+/g, ' ');
  check(linesText.includes('verified by the test'), 'a ticked acceptance line shows its note', linesText);

  sql(`update acceptance_line set note='ticked while open' where work_item_id=${id} and position=1`, true);
  for (const title of ['Work item page edited', 'Work item page']) {
    await main.getByRole('button', { name: 'Edit', exact: true }).click();
    const editDialog = page.getByRole('dialog', { name: 'Edit Work item' });
    await editDialog.getByLabel('Title').fill(title);
    await editDialog.getByRole('button', { name: 'Save' }).click();
    await editDialog.waitFor({ state: 'hidden' });
  }
  await lines.getByLabel('New acceptance line').fill('Session lasts a day');
  await lines.getByRole('button', { name: 'Add', exact: true }).click();
  await lines.getByText('Session lasts a day', { exact: true }).waitFor();
  const kept = await item();
  check(
    kept.title === 'Work item page' && kept.acceptanceLines.length === 2 && kept.acceptanceLines[0].checked === true && kept.acceptanceLines[0].note === 'ticked while open' && kept.acceptanceLines[1].checked === false,
    'editing the title and adding a line in the window keeps the lead tick and its note (REST)',
    JSON.stringify(kept.acceptanceLines),
  );
  await shot('work-item-1280x800');

  await subs.getByRole('link', { name: /Login form/ }).click();
  await main.getByRole('heading', { name: 'Login form', level: 1 }).waitFor();
  const subId = page.url().split('/').pop();
  const none = await Promise.all(['Brief', 'Sub-items', 'Proof'].map((n) => main.getByRole('region', { name: n }).count()));
  const parentLink = main.getByRole('link', { name: 'Work item page', exact: true });
  check(none.every((n) => n === 0), 'a sub-item page has no brief, sub-items or proof', JSON.stringify(none));
  check(
    (await parentLink.count()) === 1 && (await parentLink.getAttribute('href')) === `/work-items/${id}` && (await main.getByRole('region', { name: 'Acceptance lines' }).count()) === 1,
    'a sub-item page links to its parent and keeps its acceptance lines',
    `sub-item ${subId}, parent link ${await parentLink.getAttribute('href')}`,
  );
  const subHeader = (await main.getByRole('heading', { name: 'Login form', level: 1 }).locator('xpath=..').innerText()).replace(/\s+/g, ' ');
  const subSpaces = (await main.getByRole('region', { name: 'Spaces' }).innerText()).replace(/\s+/g, ' ');
  check(
    (await main.getByRole('button', { name: 'New Space' }).count()) === 0 && subHeader.includes(`work-item/${id}`) && !subHeader.includes(`work-item/${subId}`) && subSpaces.includes('proof space'),
    "a sub-item page has no New Space, shows its parent's branch and its parent's Spaces",
    `${subHeader} | ${subSpaces}`,
  );
  const subSpace = await fetch(`${base}/spaces`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ projectId: Number(pid), workItemId: Number(subId), name: 'sub space' }) });
  check(subSpace.status === 400, 'POST /spaces with a sub-item id is 400 (REST)', `${subSpace.status} ${await subSpace.text()}`);
  await shot('sub-item-1280x800');
  await parentLink.click();
  await main.getByRole('heading', { name: 'Work item page', level: 1 }).waitFor();
  check(page.url().endsWith(`/work-items/${id}`), 'the parent link opens the parent page', page.url());

  await page.setViewportSize({ width: 390, height: 844 });
  for (const [name, route] of [['parent', `/work-items/${id}`], ['sub-item', `/work-items/${subId}`]]) {
    await page.goto(base + route);
    await main.getByRole('heading', { level: 1 }).waitFor();
    await page.waitForTimeout(500);
    check(await noOverflow(), `the ${name} page has no horizontal scroll at 390x844`, `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
    const title = await main.getByRole('heading', { level: 1 }).boundingBox();
    const bar = await main.getByRole('button', { name: 'Edit', exact: true }).boundingBox();
    check(title.width >= 390 - 2 * 16 - 1 && bar.y >= title.y + title.height, `the ${name} page title uses the full width and its actions sit under it`, `title ${Math.round(title.width)} px, actions at y ${Math.round(bar.y)}`);
    await shot(`${name}-390x844`);
  }
}

async function workItemReview() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = Number(sql(`select id from project where name='${name}'`));
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    const text = await res.text();
    return text ? JSON.parse(text) : null;
  };
  const eventually = async (ok, ms = 30000) => {
    const end = Date.now() + ms;
    while (!(await ok()) && Date.now() < end) await page.waitForTimeout(200);
    return ok();
  };
  const solo = (await getJson('/teams')).find((t) => t.name === 'solo');
  const item = await call('POST', '/work-items', { projectId: pid, title: 'Review me', teamId: solo.id, acceptanceLines: [{ text: 'Wrong password is refused' }, { text: 'Session lasts a day' }] });
  const id = item.id;
  const status = async () => (await getJson(`/work-items/${id}`)).status;
  const spaceOf = async () => (await getJson(`/spaces?workItem=${id}`))[0];
  const idle = async (sid) => sql(`select active_turn from space where id=${sid}`) === '';
  const submit = async (sid) => {
    await call('POST', `/spaces/${sid}/send`, { text: 'SUBMIT FOR REVIEW' });
    await eventually(async () => (await status()) === 'review');
    await eventually(async () => (await fetch(`${base}/spaces/${sid}/proposal`)).status === 200);
    await eventually(() => idle(sid));
  };

  await call('PATCH', `/work-items/${id}`, { status: 'queued' });
  await eventually(async () => (await spaceOf()) !== undefined && (await idle((await spaceOf()).id)));
  const sid = (await spaceOf()).id;
  await submit(sid);
  const proposal = await getJson(`/spaces/${sid}/proposal`);
  check(proposal.kind === 'wrap' && proposal.items.length > 0, 'the lead submitted: the item is in review with a pending wrap-up proposal', `status ${await status()}, ${proposal.items.length} items`);

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`${base}/work-items/${id}`);
  const review = main.getByRole('region', { name: 'Review', exact: true });
  await review.waitFor();
  await review.getByText('Wrap-up proposal from lead').waitFor();
  const reviewText = (await review.innerText()).replace(/\s+/g, ' ');
  check(
    proposal.items.every((it) => reviewText.includes(it.body)) && /brief/i.test(reviewText) && (await main.getByRole('region', { name: 'Proof' }).count()) === 1,
    'the Review panel shows the wrap-up proposal next to the proof',
    reviewText.slice(0, 200),
  );
  await shot('review-1280x800');
  check(await review.getByRole('button', { name: 'Send back' }).isDisabled(), 'Send back is disabled until there is a note');

  await review.getByLabel('Note for the lead').fill('SAY IT TWICE');
  await review.getByRole('button', { name: 'Send back' }).click();
  await eventually(async () => (await getJson(`/work-items/${id}`)).status === 'in_progress');
  const woken = () => sql(`select body from message where space_id=${sid} and author='user' order by id desc limit 1`);
  await eventually(async () => woken().startsWith('Sent back from review by the user:'));
  await eventually(() => idle(sid));
  check(
    woken() === 'Sent back from review by the user:\nSAY IT TWICE' && (await status()) === 'in_progress',
    'Send back with a note puts the item back to work and the lead is woken with the note',
    `status ${await status()}, newest user message ${JSON.stringify(woken())}`,
  );
  check(sql(`select status from proposal where id=${proposal.id}`) === 'rejected', 'the wrap-up proposal it waited on is rejected', sql(`select id,status from proposal where space_id=${sid}`));

  await submit(sid);
  const again = await getJson(`/spaces/${sid}/proposal`);
  const before = Number(sql('select count(*) from memory'));
  await page.reload();
  await review.waitFor();
  await review.getByRole('button', { name: 'Done', exact: true }).click();
  await eventually(async () => (await status()) === 'done');
  await review.waitFor({ state: 'hidden' });
  const added = Number(sql('select count(*) from memory')) - before;
  check(
    (await status()) === 'done' && added === again.items.length && sql(`select status from proposal where id=${again.id}`) === 'approved' && sql(`select closed_at is not null from space where id=${sid}`) === '1',
    'one click on Done saves the wrap-up memory items, approves the proposal, closes the Space and sets Done',
    `status ${await status()}, memory rows +${added} of ${again.items.length} proposed, proposal ${sql(`select status from proposal where id=${again.id}`)}`,
  );
  const header = (await main.getByRole('heading', { name: 'Review me', level: 1 }).locator('xpath=..').innerText()).replace(/\s+/g, ' ');
  check(/done/i.test(header), 'the header shows Done', header);
  await shot('done-1280x800');

  const parent = await call('POST', '/work-items', { projectId: pid, title: 'Parent of a review' });
  const sub = await call('POST', '/work-items', { projectId: pid, parentId: parent.id, title: 'Sub in review' });
  sql(`update work_item set status='review' where id=${sub.id}`, true);
  await page.goto(`${base}/work-items/${sub.id}`);
  await main.getByRole('heading', { name: 'Sub in review', level: 1 }).waitFor();
  const subHeader = (await main.getByRole('heading', { name: 'Sub in review', level: 1 }).locator('xpath=..').innerText()).replace(/\s+/g, ' ');
  const offered = (await main.getByRole('region', { name: 'Review', exact: true }).count()) + (await main.getByRole('button', { name: 'Done', exact: true }).count());
  const res = await fetch(`${base}/work-items/${sub.id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'done' }) });
  const refusal = await res.text();
  check(
    /review/i.test(subHeader) && offered === 0 && res.status === 400 && refusal.includes('only the lead sets a sub-item done'),
    'a sub-item in review is not offered Done, and the server refuses the user setting it',
    `header ${subHeader}, Review/Done controls ${offered}, PATCH done ${res.status} ${refusal}`,
  );
  await shot('sub-item-review-1280x800');

  const bare = await call('POST', '/work-items', { projectId: pid, title: 'Review without proof' });
  sql(`update work_item set status='review' where id=${bare.id}`, true);
  await page.goto(`${base}/work-items/${bare.id}`);
  const bareReview = main.getByRole('region', { name: 'Review', exact: true });
  await bareReview.getByText('No proof submitted.').waitFor();
  await shot('no-proof-1280x800');
  check(
    (await bareReview.getByRole('button', { name: 'Send back' }).count()) === 1 && (await bareReview.getByRole('button', { name: 'Done', exact: true }).count()) === 1,
    'an item in review with no proof shows Done and Send back',
  );
  await bareReview.getByRole('button', { name: 'Done', exact: true }).click();
  await eventually(async () => (await getJson(`/work-items/${bare.id}`)).status === 'done');
  check((await getJson(`/work-items/${bare.id}`)).status === 'done', 'Done on an item with no proof sets it done');

  await page.setViewportSize({ width: 390, height: 844 });
  const rev =await call('POST', '/work-items', { projectId: pid, title: 'Review on a phone', teamId: solo.id, acceptanceLines: [{ text: 'Wrong password is refused' }, { text: 'Session lasts a day' }] });
  await call('PATCH', `/work-items/${rev.id}`, { status: 'queued' });
  await eventually(async () => (await getJson(`/spaces?workItem=${rev.id}`))[0] !== undefined);
  const rsid = (await getJson(`/spaces?workItem=${rev.id}`))[0].id;
  await eventually(() => idle(rsid));
  await call('POST', `/spaces/${rsid}/send`, { text: 'SUBMIT FOR REVIEW' });
  await eventually(async () => (await fetch(`${base}/spaces/${rsid}/proposal`)).status === 200);
  await eventually(() => idle(rsid));
  await page.goto(`${base}/work-items/${rev.id}`);
  await main.getByRole('region', { name: 'Review', exact: true }).waitFor();
  await page.waitForTimeout(500);
  const fits = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  check(fits, 'the Review panel has no horizontal scroll at 390x844', `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
  await shot('review-390x844');
}

async function spaceHeader() {
  const section = await addProject();
  const name = (await section.getAttribute('aria-label')) ?? '';
  const pid = Number(sql(`select id from project where name='${name}'`));
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  const suffix = Date.now().toString(36);
  const sam = await call('POST', '/bots', { name: `sam-${suffix}`, backend: 'claude', face: 'otter', colour: 'violet' });
  const kit = await call('POST', '/bots', { name: `kit-${suffix}`, backend: 'claude' });
  const itemA = await call('POST', '/work-items', { projectId: pid, title: `Header item ${suffix}` });
  const itemB = await call('POST', '/work-items', { projectId: pid, title: `Attach target ${suffix}` });
  const workSpace = await call('POST', '/spaces', { projectId: pid, workItemId: itemA.id, name: `item-space-${suffix}` });
  const freeSpace = await call('POST', '/spaces', { projectId: pid, name: `free-space-${suffix}` });
  const conversation = await call('POST', '/spaces', { projectId: pid, kind: 'conversation', botId: sam.id, name: `chat-${suffix}` });
  const turn = (space, input, output, read, write) =>
    sql(`insert into turn (space_id, role, backend, status, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, started_at) values (${space}, 'lead', 'claude', 'done', ${input}, ${output}, ${read}, ${write}, '2026-01-01T00:00:00Z')`, true);
  turn(workSpace.id, 1000, 100, 9999, 50);
  turn(workSpace.id, 200, 30, 0, 0);
  turn(freeSpace.id, 500, 0, 0, 0);

  const header = main.locator('header').first();
  const actions = async () => {
    const found = [];
    for (const [role, label] of [['link', 'Open Work item'], ['button', 'Attach to Work item'], ['button', 'Add Bot']]) {
      if ((await header.getByRole(role, { name: label, exact: true }).count()) > 0) found.push(label);
    }
    return found.join(', ');
  };
  const headerText = async () => (await header.innerText()).replace(/\s+/g, ' ');
  const memberNames = async (id) => (await getJson(`/spaces/${id}/members`)).map((m) => m.name);
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  const open = async (id, title) => {
    await page.goto(`${base}/spaces/${id}`);
    await main.getByRole('heading', { name: title, level: 1 }).waitFor();
  };
  const panel = page.getByRole('complementary', { name: 'Bots' });
  const members = async () => {
    await header.getByRole('button', { name: 'Bots', exact: true }).click();
    await panel.waitFor();
    const rows = panel.locator('ul').first().getByRole('listitem');
    const out = { count: await rows.count(), creatures: await rows.locator('svg').count(), text: (await rows.allInnerTexts()).join(' | ').replace(/\s+/g, ' ').toLowerCase(), leads: await panel.locator('.rounded-pill').filter({ hasText: /^coordinator$/ }).count() };
    return out;
  };

  await page.setViewportSize({ width: 1280, height: 800 });
  await open(workSpace.id, `item-space-${suffix}`);
  const used = (await getJson(`/spaces/${workSpace.id}`)).tokensUsed;
  const budget = (await getJson(`/spaces/${workSpace.id}`)).tokenBudget;
  const inspector = await getJson(`/spaces/${workSpace.id}/inspector`);
  const inspectorSum = inspector.roles.reduce((n, r) => n + r.usage.inputTokens + r.usage.outputTokens + r.usage.cacheWriteTokens, 0);
  const sqlSum = Number(sql(`select sum(input_tokens + output_tokens + cache_write_tokens) from turn where space_id=${workSpace.id}`));
  const text = await headerText();
  check(text.includes(`Header item ${suffix}`) && text.includes('backlog'), "a Work item Space's header shows the item's title and status", text);
  check(used === 1380 && used === inspectorSum && used === sqlSum, 'REST tokensUsed is the input, output and cache write tokens (cache reads not counted)', `GET ${used}, inspector ${inspectorSum}, sql ${sqlSum}`);
  check(text.includes(`Budget ${used.toLocaleString('en-US')} / ${budget.toLocaleString('en-US')}`), "the header's budget used is the REST tokensUsed over tokenBudget", text);
  check((await actions()) === 'Open Work item', 'a Work item Space has Open Work item and neither Attach nor Add Bot', await actions());
  const mWork = await members();
  check(mWork.count === (await memberNames(workSpace.id)).length && mWork.creatures >= mWork.count && mWork.leads === 1, "the member list shows one creature per member of GET /spaces/{id}/members and one coordinator badge", JSON.stringify(mWork));
  await shot('desktop-work-item');
  await page.keyboard.press('Escape');

  await open(freeSpace.id, `free-space-${suffix}`);
  const freeText = await headerText();
  check(freeText.includes('free Space') && !freeText.includes('Budget'), 'a free Space header names its kind and has no budget chip', freeText);
  check((await actions()) === 'Attach to Work item, Add Bot', 'a free Space has Attach to Work item and Add Bot, no Open Work item', await actions());
  await header.getByRole('button', { name: 'Add Bot', exact: true }).click();
  const addDialog = page.getByRole('dialog', { name: 'Add a Bot' });
  await addDialog.getByRole('button', { name: kit.name, exact: true }).waitFor();
  check((await addDialog.getByRole('button', { name: 'lead', exact: true }).count()) === 0, "the picker leaves out the Space's own members");
  await shot('desktop-free-add-teammate');
  await addDialog.getByRole('button', { name: kit.name, exact: true }).click();
  await addDialog.waitFor({ state: 'hidden' });
  const after = await memberNames(freeSpace.id);
  check(after.join() === `lead,${kit.name}`, 'Add Bot on a free Space puts the Bot in GET /spaces/{id}/members', after.join());
  const mFree = await members();
  check(mFree.count === 2 && mFree.text.includes(kit.name) && mFree.leads === 1, 'the member list now shows both members', JSON.stringify(mFree));
  await shot('desktop-free');
  await page.keyboard.press('Escape');

  await open(conversation.id, `chat-${suffix}`);
  const convText = await headerText();
  const face = await header.getByRole('button', { name: 'Show Bots' }).locator('svg').first().innerHTML();
  check(convText.includes('conversation') && !convText.includes('Budget'), 'a conversation header names its kind and has no budget chip', convText);
  check(face.includes('#8B5CF6') && !face.includes('#8E8E93'), "a conversation header shows the Bot's creature in its colour, not a pebble");
  check((await actions()) === 'Add Bot', 'a conversation has Add Bot only', await actions());
  const mConv = await members();
  check(mConv.count === 1 && mConv.text.includes(sam.name), 'the conversation lists its one Bot', JSON.stringify(mConv));
  await shot('desktop-conversation');
  await page.keyboard.press('Escape');

  await page.setViewportSize({ width: 390, height: 844 });
  for (const [id, title, step] of [[workSpace.id, `item-space-${suffix}`, 'phone-work-item'], [freeSpace.id, `free-space-${suffix}`, 'phone-free'], [conversation.id, `chat-${suffix}`, 'phone-conversation']]) {
    await open(id, title);
    check(await noOverflow(), `${step} has no horizontal scroll at 390x844`, `${await page.evaluate(() => document.documentElement.scrollWidth)} px`);
    await shot(step);
  }
  await page.setViewportSize({ width: 1280, height: 800 });

  await open(freeSpace.id, `free-space-${suffix}`);
  await header.getByRole('button', { name: 'Attach to Work item', exact: true }).click();
  const attachDialog = page.getByRole('dialog', { name: 'Attach to a Work item' });
  await pick('Work item', itemB.title, attachDialog);
  await attachDialog.getByRole('button', { name: 'Attach', exact: true }).click();
  await header.getByRole('link', { name: 'Open Work item', exact: true }).waitFor();
  const attached = await getJson(`/spaces/${freeSpace.id}`);
  check(attached.workItemId === itemB.id && attached.kind === 'work_item', 'Attach from the header makes the Space a Work item Space of the item', `workItemId ${attached.workItemId}, kind ${attached.kind}`);
  const attachedText = await headerText();
  check(attachedText.includes(itemB.title) && attachedText.includes(`Budget ${attached.tokensUsed.toLocaleString('en-US')} /`) && attached.tokensUsed === 500, "the attached Space's header shows the item and its budget used", attachedText);
  check((await actions()) === 'Open Work item', 'the attached Space offers Open Work item only', await actions());
  await shot('desktop-attached');
  await header.getByRole('link', { name: 'Open Work item', exact: true }).click();
  await page.waitForURL(`**/work-items/${itemB.id}`);
  const spaces = main.getByRole('region', { name: 'Spaces' });
  await spaces.getByRole('link', { name: new RegExp(`free-space-${suffix}`) }).waitFor();
  check((await getJson(`/spaces?workItem=${itemB.id}`)).map((s) => s.id).join() === String(freeSpace.id), "the Space is on the item's page and in GET /spaces?workItem=", `${itemB.id}`);
  await shot('desktop-item-page');
}

async function globalChats() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const suffix = Date.now().toString(36);
  const call = async (method, route, body) => {
    const res = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  await addProject();
  const a = (await getJson('/projects')).at(-1);
  const otherDir = mkdtempSync(path.join(tmpdir(), 'reef-ui-other-'));
  const b = await call('POST', '/projects', { name: `other-${suffix}`, path: otherDir });
  const bot = await call('POST', '/bots', { name: `chatter-${suffix}`, backend: 'claude', write: false });
  const botPage = `${base}/bots/${bot.id}`;
  const resume = main.getByRole('link', { name: 'Resume chat' });
  const newChat = main.getByRole('button', { name: 'New chat' });
  const spaceCount = async () => (await getJson('/spaces')).length;
  const filter = async (name) => {
    await aside.getByRole('button', { name: 'Switch project' }).click();
    await page.getByRole('menuitem', { name, exact: true }).click();
    await aside.getByRole('region', { name }).waitFor();
  };
  const startChat = async () => {
    const was = page.url();
    await newChat.click();
    await page.waitForURL((u) => u.href !== was && /\/spaces\/\d+$/.test(u.pathname));
    return Number(page.url().split('/').pop());
  };
  const state = async (id) => ({
    space: { ...(await getJson(`/spaces/${id}`)), tokensUsed: undefined },
    projects: (await getJson(`/spaces/${id}/projects`)).map((p) => p.id),
    sessions: [...new Set((await getJson(`/turns?space=${id}`)).map((t) => t.sessionId))],
    messages: (await getJson(`/messages?space=${id}`)).length,
  });

  await filter('All projects');
  check((await aside.getByRole('region', { name: 'All projects' }).count()) === 1, 'the project switcher offers All projects and the sidebar names it');
  await page.goto(botPage);
  await main.getByRole('heading', { name: bot.name, level: 1 }).waitFor();
  await newChat.waitFor();
  check((await resume.count()) === 0, 'under All projects a Bot with no chat offers only New chat');
  const first = await startChat();
  check((await state(first)).projects.length === 0 && (await getJson(`/spaces/${first}`)).projectId === 0, 'New chat under All projects starts unassociated (no projects, projectId 0)', JSON.stringify((await state(first)).projects));
  await exchange('Describe a reef.');
  const dir = path.join(data, 'chats', String(first));
  const repo = (d) => { try { execFileSync('git', ['-C', d, 'rev-parse', '--is-inside-work-tree'], { stdio: 'pipe' }); return true; } catch { return false; } };
  let ancestor = '';
  for (let d = dir; d !== path.dirname(d); d = path.dirname(d)) if (existsSync(path.join(d, '.git'))) ancestor = d;
  check(existsSync(dir) && !repo(dir) && ancestor === '', 'the chat ran in its own folder under the data folder: not a git repo, no .git in it or above', `${dir} ancestor .git: ${ancestor || 'none'}`);
  const sessions = (await state(first)).sessions;
  const logged = (process.env.REEF_FAKE_LOG ? readFileSync(process.env.REEF_FAKE_LOG, 'utf8') : '').split('\n').filter((l) => sessions.some((s) => l.includes(`session=${s} `)));
  check(logged.length === 1 && logged[0].includes(` cwd=${dir} `), 'the agent process started in that folder (fake claude log)', logged[0]?.split(' args=')[0]);
  const prompt = (await getJson(`/spaces/${first}/inspector`)).turns[0].prompt;
  check(![a.path, b.path, 'Base branch'].some((x) => prompt.includes(x)), 'its first prompt has no project path and no base branch', prompt.split('\n').slice(0, 3).join(' / '));
  await shot('all-projects-chat');

  await page.goto(botPage);
  await resume.waitFor();
  const before = await spaceCount();
  check((await resume.getAttribute('href')) === `/spaces/${first}`, 'reopening the Bot page offers Resume chat for the latest chat and starts nothing', `${await resume.getAttribute('href')} spaces ${before}`);
  await page.waitForTimeout(1000);
  check((await spaceCount()) === before, 'no chat was started by opening the page', String(await spaceCount()));
  const second = await startChat();
  await exchange('Describe another reef.');
  const list = (await getJson(`/bots/${bot.id}/conversations`)).map((c) => c.id);
  check(second !== first && list.includes(first) && list.includes(second), 'New chat made a separate chat and the older one is still listed', `${first} and ${second}: ${list.join()}`);
  const ids = await Promise.all([first, second].map(async (id) => (await state(id)).sessions[0]));
  check(ids[0] !== ids[1] && (await state(first)).messages === 2, 'the two chats have their own sessions and the older one kept its two messages', ids.join(' vs '));
  await shot('new-chat-second');

  await filter(a.name);
  check((await getJson(`/spaces?project=${a.id}`)).length === 0, 'GET /spaces?project= lists neither global chat under a Project they are not associated with');
  await page.goto(botPage);
  await newChat.waitFor();
  check((await resume.count()) === 0, 'under a Project with no associated chat the Bot offers only New chat');
  const third = await startChat();
  const thirdProjects = (await state(third)).projects;
  check(thirdProjects.join() === String(a.id), 'New chat under a Project is associated with it at once', JSON.stringify(thirdProjects));
  await exchange('Describe a reef near the project.');
  const thirdSession = (await state(third)).sessions[0];
  const thirdLog = (process.env.REEF_FAKE_LOG ? readFileSync(process.env.REEF_FAKE_LOG, 'utf8') : '').split('\n').find((l) => l.includes(`session=${thirdSession} `));
  check(thirdLog?.includes(` cwd=${path.join(data, 'chats', String(third))} `) === true, 'it runs in a folder of its own too, never the project folder', thirdLog?.split(' args=')[0]);
  await page.goto(botPage);
  await resume.waitFor();
  check((await resume.getAttribute('href')) === `/spaces/${third}`, 'under the Project, Resume chat opens the latest associated chat, not the global ones', await resume.getAttribute('href'));

  await page.goto(`${base}/spaces/${first}`);
  await main.getByRole('heading', { level: 1 }).waitFor();
  const url = page.url();
  const was = await state(first);
  const count = await spaceCount();
  for (const name of [b.name, a.name, 'All projects']) await filter(name);
  const now = await state(first);
  check(JSON.stringify(was) === JSON.stringify(now), 'switching the Project filter changes nothing of the reopened chat: space, associations, sessions, messages', `projects ${JSON.stringify(now.projects)} sessions ${now.sessions.join()}`);
  check(page.url() === url && (await spaceCount()) === count, 'the page stays on the chat and starts no other chat', page.url());
  await shot('filter-switched');
}

async function navigate() {
  await page.setViewportSize({ width: 1280, height: 800 });
  const suffix = Date.now().toString(36);
  await addProject();
  const project = (await getJson('/projects')).at(-1);
  const make = async (route, body) => {
    const res = await sendJson('POST', route, body);
    if (res.status !== 201) throw new Error(`POST ${route}: ${res.status} ${JSON.stringify(res.body)}`);
    return res.body;
  };
  const sam = await make('/bots', { name: `sam-${suffix}`, backend: 'claude' });
  const kit = await make('/bots', { name: `kit-${suffix}`, backend: 'claude' });
  const chat = await make('/spaces', { projectId: project.id, name: `chat-${suffix}`, botId: sam.id, kind: 'conversation' });
  const room = await make('/spaces', { projectId: project.id, name: `room-${suffix}`, botIds: [sam.id, kit.id], coordinatorBotId: sam.id });
  const item = await make('/work-items', { projectId: project.id, title: `item-${suffix}` });
  const pins = async () => (await getJson('/pins')).map((p) => `${p.kind}:${p.id}`).sort().join();
  const spaceCount = async () => (await getJson('/spaces')).length;
  await page.goto(base + '/');
  const rowOf = (name) => aside.locator('div.group').filter({ hasText: name });

  // Pin one of each kind: the Bot page, a chat row and a Space row.
  await page.goto(`${base}/bots/${sam.id}`);
  await main.getByRole('heading', { name: sam.name, level: 1 }).waitFor();
  await main.getByRole('button', { name: 'Pin', exact: true }).first().click();
  for (const name of [chat.name, room.name]) {
    const row = rowOf(name);
    await row.first().hover();
    await row.first().getByRole('button', { name: 'Pin', exact: true }).click();
  }
  const want = [`bot:${sam.id}`, `chat:${chat.id}`, `space:${room.id}`].sort().join();
  for (let i = 0; i < 20 && (await pins()) !== want; i++) await page.waitForTimeout(250);
  check((await pins()) === want, 'a Bot, a chat and a Space each pin through their own button (GET /pins)', await pins());
  const pinned = aside.getByRole('region', { name: 'Pinned' });
  await pinned.waitFor();
  const labels = (await pinned.getByRole('listitem').allInnerTexts()).map((t) => t.replace(/\s+/g, ' '));
  check(labels.length === 3 && [sam.name, chat.name, room.name].every((n) => labels.some((l) => l.includes(n))) && ['Bot', 'Chat', 'Space'].every((k) => labels.some((l) => l.endsWith(k))), 'the Pinned section lists the three with their kind', labels.join(' | '));
  await page.reload();
  await aside.getByRole('region', { name: 'Pinned' }).waitFor();
  check((await aside.getByRole('region', { name: 'Pinned' }).getByRole('listitem').count()) === 3, 'the pins survive a reload');
  await shot('pinned');

  // Search finds all four kinds by name; a result opens its resource.
  const search = aside.getByRole('searchbox', { name: 'Search' });
  await search.fill(suffix);
  const results = aside.getByRole('region', { name: 'Search results' });
  await results.getByRole('listitem').nth(4).waitFor();
  const found = (await results.getByRole('listitem').allInnerTexts()).map((t) => t.replace(/\s+/g, ' '));
  check(found.length === 5 && [[sam.name, 'Bot'], [chat.name, 'Chat'], [room.name, 'Space'], [item.title, 'Work item']].every(([n, k]) => found.some((f) => f.includes(n) && f.endsWith(k))), 'search finds a Bot, a chat, a Space and a Work item', found.join(' | '));
  await shot('search');
  await results.getByRole('link', { name: new RegExp(item.title) }).click();
  await page.waitForURL((u) => u.pathname === `/work-items/${item.id}`);
  check(true, 'a Work item result opens its page');

  // A Bot with no chat opens its page, and only New chat starts one.
  await search.fill(kit.name);
  await results.getByRole('link', { name: new RegExp(kit.name) }).click();
  await page.waitForURL((u) => u.pathname === `/bots/${kit.id}`);
  const before = await spaceCount();
  await main.getByRole('button', { name: 'New chat' }).waitFor();
  await page.waitForTimeout(1000);
  check((await main.getByRole('link', { name: 'Resume chat' }).count()) === 0 && (await spaceCount()) === before, 'a Bot with no chat opens its page with New chat and no chat is created', `spaces ${before} -> ${await spaceCount()}`);
  await search.fill(sam.name);
  await results.getByRole('link', { name: new RegExp(sam.name) }).click();
  await page.waitForURL((u) => u.pathname === `/spaces/${chat.id}`);
  check(true, "a Bot with a chat opens that chat");

  // Unpin removes the row.
  await page.goto(`${base}/bots/${sam.id}`);
  await main.getByRole('button', { name: 'Unpin', exact: true }).first().click();
  for (let i = 0; i < 20 && (await pins()).includes('bot:'); i++) await page.waitForTimeout(250);
  check(!(await pins()).includes('bot:') && (await pins()).split(',').length === 2, 'Unpin removes the Bot pin and keeps the other two', await pins());

  // The coordinator changes without changing anything else of the Space.
  const members = async () => (await getJson(`/spaces/${room.id}/members`)).map((m) => `${m.name}${m.lead ? '*' : ''}`).sort().join();
  check((await members()) === `${kit.name},${sam.name}*`, 'the Space starts with the coordinator it was given', await members());
  const set = await sendJson('PUT', `/spaces/${room.id}/coordinator`, { botId: kit.id });
  check(set.status === 200 && (await members()) === `${kit.name}*,${sam.name}`, 'PUT /spaces/{id}/coordinator moves the coordinator and keeps both members', `${set.status} ${await members()}`);
  const outsider = await make('/bots', { name: `out-${suffix}`, backend: 'claude' });
  check((await sendJson('PUT', `/spaces/${room.id}/coordinator`, { botId: outsider.id })).status === 400, 'a Bot that is not a member cannot be made coordinator');
}

try {
  await scenarios[scenario]();
} catch (err) {
  check(false, 'scenario aborted', String(err.message).split('\n').slice(0, 3).join(' | '));
  await shot('error').catch(() => {});
}
await browser.close();
process.exit(failed ? 1 : 0);

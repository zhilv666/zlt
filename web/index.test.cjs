const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');

const html = fs.readFileSync(path.join(__dirname, 'index.html'), 'utf8');
const scripts = [...html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/g)]
  .filter((match) => !/\bsrc\s*=/.test(match[1]))
  .map((match) => match[2]);
const main = scripts.find((script) => script.includes('function parseArgs('));

function pageFunctions() {
  const context = vm.createContext({});
  // Load the actual page functions without starting its network polling.
  for (const name of ['parseArgs', 'formatArgs', 'parseEnvLines', 'formatEnvLines', 'openEditTaskModal', 'saveTask']) {
    const start = main.indexOf(`function ${name}(`);
    assert.ok(start >= 0);
    const end = main.indexOf('\n    }', start) + '\n    }'.length;
    const prefix = name === 'saveTask' ? 'async ' : '';
    vm.runInContext(prefix + main.slice(start, end), context);
  }
  return context;
}

test('all inline scripts compile', () => {
  for (const script of scripts) new vm.Script(script);
});

test('parameters round trip including spaces, quotes, empty strings and backslashes', () => {
  const page = pageFunctions();
  const args = ['--config', 'C:\\App Data\\config.json', '', 'a"b', 'line\nbreak', '\\', '中文'];
  assert.deepEqual(JSON.parse(JSON.stringify(page.parseArgs(page.formatArgs(args)))), args);
  assert.deepEqual(JSON.parse(JSON.stringify(page.parseArgs(''))), []);
});

test('invalid parameters are rejected', () => {
  const page = pageFunctions();
  for (const input of ['server --port 5244', '{}', 'null', '[1]', '["valid", null]']) {
    assert.throws(() => page.parseArgs(input));
  }
});

test('editing a name and saving keeps imported parameter boundaries', async () => {
  const page = pageFunctions();
  const fields = new Map();
  page.document = {
    getElementById(id) {
      if (!fields.has(id)) fields.set(id, { value: '', checked: false, innerHTML: '', disabled: false });
      return fields.get(id);
    }
  };
  page.tasksById = new Map();
  const task = { id: 'demo', name: 'Demo', program: 'python', args: ['--config', 'C:\\App Data\\config.json', ''] };
  page.tasksById.set(task.id, task);
  page.currentTasks = [task];
  page.editingTaskId = null;
  page.openModal = () => {};
  page.escapeHTML = (value) => String(value);
  page.closeModal = () => {};
  page.loadTasks = async () => {};
  page.alert = (message) => { throw new Error(message); };
  let saved;
  page.api = async (_url, options) => { saved = JSON.parse(options.body); return { code: 0 }; };
  page.openEditTaskModal(task);
  fields.get('task-name').value = 'Renamed';
  await page.saveTask();
  assert.equal(saved.name, 'Renamed');
  assert.deepEqual(saved.args, task.args);
});

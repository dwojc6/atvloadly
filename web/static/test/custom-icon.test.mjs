import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../src/components/CustomIconPicker.vue', import.meta.url), 'utf8');
const script = source.match(/<script>([\s\S]*?)<\/script>/)[1].replace(/^import.*;$/gm, '').replace('export default', 'globalThis.component =');
function picker(decode = async () => {}, api = {}) {
  const events = [];
  const context = vm.createContext({
    api,
    FileReader: class { readAsDataURL(file) { this.result = file.value; this.onload(); } },
    Image: class { naturalWidth = 800; naturalHeight = 480; decode = decode; },
  });
  vm.runInContext(script, context);
  const component = context.component;
  const state = Object.assign(component.data(), {
    $emit: (...event) => events.push(event),
    $t: (key) => key,
    $el: { querySelector: () => ({ value: 'selected' }) },
  });
  for (const [name, method] of Object.entries(component.methods)) state[name] = method.bind(state);
  return { state, events };
}
const selected = (file = { type: 'image/png', size: 100, value: 'data:image/png;base64,TEST' }) => ({ target: { files: [file], value: 'selected' } });

test('selects valid icon artwork and releases the install button', async () => {
  const { state, events } = picker();
  await state.selectIcon(selected());
  assert.deepEqual(events, [['busy', true], ['update:modelValue', 'data:image/png;base64,TEST'], ['busy', false]]);
});

test('rejects unsupported and oversized artwork without replacing the current icon', async () => {
  for (const file of [{ type: 'image/svg+xml', size: 1 }, { type: 'image/png', size: 5 * 1024 * 1024 }]) {
    const { state, events } = picker();
    await state.selectIcon(selected(file));
    assert.match(state.error, /invalid/);
    assert.equal(events.some(([name]) => name === 'update:modelValue'), false);
    assert.deepEqual(events.at(-1), ['busy', false]);
  }
});

test('reset prevents a pending image decode from restoring the override', async () => {
  let finish;
  const { state, events } = picker(() => new Promise((resolve) => { finish = resolve; }));
  const pending = state.selectIcon(selected());
  await new Promise(setImmediate);
  state.clearIcon();
  finish();
  await pending;
  assert.deepEqual(events.filter(([name]) => name === 'update:modelValue'), [['update:modelValue', '']]);
});

test('searches Apple by app name in the selected TV storefront', async () => {
  let parameters;
  const { state } = picker(undefined, {
    searchAppleIcons: async (params) => { parameters = params; return { data: [{ id: 123, name: 'Plex' }] }; },
  });
  state.query = ' Plex ';
  state.country = 'GB';
  state.isTv = true;
  await state.searchApple();
  assert.equal(parameters.term, 'Plex');
  assert.equal(parameters.country, 'GB');
  assert.equal(parameters.platform, 'tvos');
  assert.equal(state.results[0].name, 'Plex');
  assert.equal(state.searching, false);
  assert.equal(state.searched, true);
});

test('typing a different app name discards late search results', async () => {
  let finish;
  const { state } = picker(undefined, { searchAppleIcons: () => new Promise((resolve) => { finish = resolve; }) });
  state.query = 'Plex';
  const pending = state.searchApple();
  state.query = 'Infuse';
  state.invalidateSearch();
  finish({ data: [{ id: 123, name: 'Plex' }] });
  await pending;
  assert.equal(state.results.length, 0);
  assert.equal(state.searched, false);
});

test('stores downloaded Apple artwork through the existing custom icon field', async () => {
  const { state, events } = picker(undefined, { downloadAppleIcon: async () => ({ data: 'data:image/png;base64,APPLE' }) });
  await state.chooseAppleIcon({ id: 123, artwork_url: 'https://is1.mzstatic.com/icon.jpg' });
  assert.deepEqual(events, [['busy', true], ['update:modelValue', 'data:image/png;base64,APPLE'], ['busy', false]]);
});

test('reset discards an Apple artwork download still in flight', async () => {
  let finish;
  const { state, events } = picker(undefined, { downloadAppleIcon: () => new Promise((resolve) => { finish = resolve; }) });
  const pending = state.chooseAppleIcon({ id: 123, artwork_url: 'https://is1.mzstatic.com/icon.jpg' });
  state.clearIcon();
  finish({ data: 'data:image/png;base64,APPLE' });
  await pending;
  assert.deepEqual(events.filter(([name]) => name === 'update:modelValue'), [['update:modelValue', '']]);
  assert.equal(state.selecting, 0);
});

test('an Apple outage keeps the selected icon and releases busy state', async () => {
  const { state, events } = picker(undefined, { downloadAppleIcon: async () => { throw new Error('Apple is unavailable'); } });
  await state.chooseAppleIcon({ id: 123 });
  assert.equal(state.error, 'Apple is unavailable');
  assert.equal(events.some(([name]) => name === 'update:modelValue'), false);
  assert.deepEqual(events.at(-1), ['busy', false]);
});

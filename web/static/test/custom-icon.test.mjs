import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../src/components/CustomIconPicker.vue', import.meta.url), 'utf8');
const script = source.match(/<script>([\s\S]*?)<\/script>/)[1].replace('export default', 'globalThis.component =');
function picker(decode = async () => {}) {
  const events = [];
  const context = vm.createContext({
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

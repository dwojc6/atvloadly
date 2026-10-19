import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../src/page/laboratory/index.vue',import.meta.url),'utf8');
const script=source.match(/<script>([\s\S]*?)<\/script>/)[1].replace(/^import.*;$/gm,'').replace('export default','globalThis.component =');
function page(scanWireless = async () => ({data:[]})) {
 const submitted=[],errors=[];
 const context=vm.createContext({
  api:{scanWireless,importPair:async (data)=>submitted.push(data)},
  toast:{success(){},error:(message)=>errors.push(message)},
  FormData,console,
  document:{getElementById:()=>({reset(){}})},
 });
 vm.runInContext(script,context);
 const component=context.component;
 const state=component.data();
 for(const [name,method] of Object.entries(component.methods))state[name]=method.bind(state);
 state.$t=(key)=>key;state.validateForm=()=>true;
 state.files=[new Blob(['synthetic pairing file'])];
 return {state,submitted,errors,component};
}

test('imports by address when discovery returns no devices',async()=>{
 const {state,submitted}=page();
 await state.loadDevices();
 state.deviceIP='192.168.1.7';state.devicePort='49152';
 await state.onSubmit();
 assert.equal(submitted.length,1);
 assert.equal(submitted[0].get('ip'),'192.168.1.7');
 assert.equal(submitted[0].get('port'),'49152');
 assert.ok(submitted[0].get('file'));
 assert.equal(state.deviceIP,'');
 assert.equal(state.loading,false);
});

test('selecting a discovered phone fills its address and advertised port',()=>{
 const {state,component}=page();
 state.devices=[{id:'phone',ip:'192.168.1.7',port:50000}];
 component.watch.selectedDevice.call(state,'phone');
 assert.equal(state.deviceIP,'192.168.1.7');
 assert.equal(state.devicePort,'50000');
});

test('rejects invalid addresses and ports before sending a pairing file',async()=>{
 for(const [ip,port] of [['192.168.1.999','49152'],['example.com','49152'],['192.168.1.7','0'],['192.168.1.7','65536'],['192.168.1.7','1.5']]){
  const {state,submitted,errors}=page();
  state.deviceIP=ip;state.devicePort=port;
  await state.onSubmit();
  assert.equal(submitted.length,0);
  assert.equal(errors[0],'laboratory.toast.invalid_address');
 }
});

test('an unsuccessful scan does not prevent a manual import',async()=>{
 const {state,submitted}=page(async()=>{throw new Error('scan timed out');});
 await state.loadDevices();
 state.deviceIP='192.168.1.7';state.devicePort='49152';
 await state.onSubmit();
 assert.equal(submitted.length,1);
 assert.equal(state.loadingDevices,false);
});

test('does not start overlapping discovery scans',async()=>{
 let finish,calls=0;
 const {state}=page(()=>{calls++;return new Promise(resolve=>{finish=resolve;});});
 const pending=state.loadDevices();
 await state.loadDevices();
 assert.equal(calls,1);
 finish({data:[]});await pending;
});

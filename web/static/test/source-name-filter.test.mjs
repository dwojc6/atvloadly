import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import test from "node:test";
import vm from "node:vm";
import * as helpers from "../src/utils/source.mjs";

function component(file, globals = {}, props = {}) {
 const source = readFileSync(new URL(`../src/components/${file}.vue`, import.meta.url), "utf8");
 const script = source.match(/<script>([\s\S]*?)<\/script>/)[1]
  .replace(/^import[\s\S]*?;\n/gm, "")
  .replace("export default", "globalThis.component =");
 const context = vm.createContext({...helpers, console: {error(){}}, ...globals});
 vm.runInContext(script,context);
 const c=context.component;
 const state={...props,...c.data.call(props)};
 for(const [name,method] of Object.entries(c.methods))state[name]=method.bind(state);
 for(const [name,getter] of Object.entries(c.computed))Object.defineProperty(state,name,{get:typeof getter==='function'?getter.bind(state):getter.get.bind(state)});
 state.$emit=()=>{};
 return state;
}
const builds=[
 {id:'1',name:'TikTok (Infinitok) [@ipavault]',bundle_id:'com.example.tiktok',filter:'com.example.tiktok'},
 {id:'2',name:'TikTok Other',bundle_id:'com.example.tiktok',filter:'com.example.tiktok'}
];
const preview={kind:'altstore',url:'https://example.com/repo.json',builds};

test('AltStore picker emits a saved name regex along with the bundle identifier',()=>{
 const picker=component('SourcePicker',{}, {initial:null});
 picker.applyPreview(preview);
 assert.equal(picker.selection,null);
 picker.selectBuild('1');
 assert.equal(picker.selection.filter,'com.example.tiktok');
 assert.equal(helpers.nameMatches(picker.selection.name_filter,builds[0]),true);
 assert.equal(helpers.nameMatches(picker.selection.name_filter,builds[1]),false);
});

test('reopening an AltStore source preserves a custom name regex',()=>{
 const picker=component('SourcePicker',{}, {initial:{filter:'com.example.tiktok',name_filter:'(?i)infinitok'}});
 picker.applyPreview(preview);
 assert.equal(picker.selectedId,'1');
 assert.equal(picker.selection.name_filter,'(?i)infinitok');
 picker.nameFilter='(';picker.nameFilterCustom=true;
 assert.equal(picker.nameFilterMismatch,true);
 picker.resetNameFilter();
 assert.equal(picker.nameFilterMismatch,false);
 assert.equal(picker.selection.name_filter,helpers.deriveNameFilter(builds[0].name));
});

test('an update sends the edited regex while preserving the installed regex until success',async()=>{
 const links=[],updates=[];
 const app={ID:1,source:{kind:'altstore',url:preview.url,filter:'com.example.tiktok',name_filter:'^Old$',auto_update:false}};
 const dialog=component('SourceDialog',{SourcePicker:{},api:{
  linkAppSource:async(id,payload)=>{links.push(payload);return {data:app};},
  updateAppFromSource:async(id,payload)=>{updates.push(payload);throw new Error('install failed');}
 }});
 dialog.app=app;
 dialog.autoUpdate=true;
 dialog.selection={kind:'altstore',url:preview.url,filter:'com.example.tiktok',name_filter:'^New$',prerelease:false,build:{id:'new'}};
 await dialog.install();
 assert.equal(links[0].name_filter,'^Old$');
 assert.equal(updates[0].name_filter,'^New$');
 assert.equal(dialog.app.source.name_filter,'^Old$');
});

test('GitHub search reaches IPA assets beyond the initial list and searches release tags',()=>{
 const picker=component('SourcePicker',{}, {initial:{kind:'github',prerelease:true}});
 const builds=Array.from({length:100},(_,i)=>({id:String(i+1),name:'TwitchUnblock.ipa',version:`nightly-${327-i}`,filter:'^TwitchUnblock\\.ipa$'}));
 builds.push({id:'tv',name:'TwitchUnblockTV.ipa',version:'tv-test-27',filter:'^TwitchUnblockTV\\.ipa$',prerelease:true});
 picker.applyPreview({kind:'github',url:'MXFia19/TwitchUnblock',builds,suggested_id:'1'});
 assert.equal(picker.searchable,true);
 assert.equal(picker.matches.length,101);
 assert.equal(picker.hiddenCount,51);
 picker.query='TwitchUnblockTV';
 assert.equal(picker.visibleBuilds.length,1);
 assert.equal(picker.visibleBuilds[0].id,'tv');
 picker.selectBuild('tv');
 assert.equal(picker.selection.build.version,'tv-test-27');
 assert.equal(picker.selection.prerelease,true);
 picker.query='tv-test-27';
 assert.equal(picker.visibleBuilds[0].id,'tv');
});

test('GitHub history reopens the newest matching saved variant instead of the recommendation',()=>{
 const picker=component('SourcePicker',{}, {initial:{kind:'github',filter:'^TwitchUnblockTV\\.ipa$',prerelease:true}});
 picker.applyPreview({kind:'github',url:'MXFia19/TwitchUnblock',suggested_id:'ios',builds:[
  {id:'ios',name:'TwitchUnblock.ipa',version:'nightly-327',filter:'^TwitchUnblock\\.ipa$'},
  {id:'tv27',name:'TwitchUnblockTV.ipa',version:'tv-test-27',filter:'tv'},
  {id:'tv26',name:'TwitchUnblockTV.ipa',version:'tv-test-26',filter:'tv'}
 ]});
 assert.equal(picker.selectedId,'tv27');
 assert.equal(picker.selection.filter,'^TwitchUnblockTV\\.ipa$');
});

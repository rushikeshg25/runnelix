import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { executeNode, parseEnvelope, NodeFailure } from './product-host.js';
const fixtures: {name:string;value:Record<string,unknown>;valid:boolean}[] = JSON.parse(readFileSync('contracts/product/activity.fixtures.json','utf8'));
for (const fixture of fixtures) test(`product envelope: ${fixture.name}`, () => {
  if(fixture.valid) assert.doesNotThrow(()=>parseEnvelope(fixture.value));
  else assert.throws(()=>parseEnvelope(fixture.value),NodeFailure);
});
const catalog = JSON.parse(readFileSync('contracts/product/catalog.json','utf8'));
const task = {...fixtures[0]!.value, implementationDigest: catalog[1].implementationDigest};
test('mock module resolves scoped references and preserves data',async()=>{
 const refs:string[]=[];const input={unicode:'こんにちは',nested:[1,null,true],large:'9007199254740993'};
 const result=await executeNode(task,{signal:new AbortController().signal,resolve:async(workspace,run,ref)=>{assert.equal(workspace,'workspace_test');assert.equal(run,'run_test');refs.push(ref);return ref==='payload_input'?input:{}}});
 assert.deepEqual(result,input);assert.notEqual(result,input);assert.deepEqual(refs,['payload_input','payload_config']);
});
test('wrong artifact fails before resolution',async()=>{await assert.rejects(executeNode({...task,implementationDigest:'sha256:'+'1'.repeat(64)},{signal:new AbortController().signal,resolve:async()=>{assert.fail('must not resolve')}}),{code:'VersionMismatch'})});
test('resolver failures are sanitized',async()=>{await assert.rejects(executeNode(task,{signal:new AbortController().signal,resolve:async()=>{throw Error('sentinel-secret')}}),{code:'PayloadUnavailable',message:'Scoped payload unavailable'})});
test('invalid config, oversized input, and cancellation are rejected',async()=>{
 for(const data of [{config:{unexpected:true},input:{}},{config:{},input:{value:'x'.repeat(8193)}}]){
 await assert.rejects(executeNode(task,{signal:new AbortController().signal,resolve:async(_w,_r,ref)=>ref==='payload_input'?data.input:data.config}),{code:'InvalidInput'});}
 const c=new AbortController();c.abort();await assert.rejects(executeNode(task,{signal:c.signal,resolve:async()=>assert.fail('cancelled before resolve')}),{name:'AbortError'});
});

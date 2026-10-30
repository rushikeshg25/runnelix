import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { executeNode } from './product-host.js';
import type { ApprovedRequest } from './nodes/index.js';
const catalog: {type:string;implementationDigest:string}[] = JSON.parse(readFileSync('contracts/product/catalog.json','utf8'));
const fixture = JSON.parse(readFileSync('contracts/product/activity.fixtures.json','utf8'))[0].value;
function invoke(type: string, input: unknown, config: unknown, request: (r: ApprovedRequest) => Promise<unknown>, signal=new AbortController().signal) {
 const task = {...fixture,nodeType:type,nodeVersion:1,implementationDigest:catalog.find(d=>d.type===type)!.implementationDigest};
 return executeNode(task,{signal,resolve:async(_w,_r,ref)=>ref===task.inputRef?input:config,capabilities:{request}});
}
const github={owner:'fixture-team',repository:'example.repo',credentialRef:'credential_fixture'};
test('GitHub issue adapter uses fixed route and emits bounded identifiers',async()=>{
 const input={title:'Fixture only',body:'Example'};
 const output=await invoke('github.createIssue',input,github,async request=>{
  assert.deepEqual(request,{url:'https://api.github.com/repos/fixture-team/example.repo/issues',method:'POST',body:input});
  assert.ok(!JSON.stringify(request).includes('credential_fixture'));
  return {status:201,body:{id:123,number:7,html_url:'https://github.com/fixture-team/example.repo/issues/7',ignored:'not persisted'}};
 });assert.deepEqual(output,{id:123,number:7,url:'https://github.com/fixture-team/example.repo/issues/7'});
});
test('GitHub validates path config and input before network access',async()=>{
 for(const config of [{...github,owner:'../other'},{...github,repository:'../escape'},{owner:'team',repository:'repo'}]) await assert.rejects(invoke('github.createIssue',{title:'test'},config,async()=>assert.fail('must not send')),{code:'InvalidInput'});
 await assert.rejects(invoke('github.createIssue',{},github,async()=>assert.fail('must not send')),{code:'InvalidInput'});
});
test('GitHub malformed successful response is not accepted as success',async()=>{
 await assert.rejects(invoke('github.createIssue',{title:'test'},github,async()=>({status:201,body:{id:'wrong'}})),{code:'InvalidOutput'});
 const controller=new AbortController();controller.abort();
 await assert.rejects(invoke('github.createIssue',{title:'test'},github,async()=>assert.fail('cancelled before send'),controller.signal),{name:'AbortError'});
});
const slack={channel:'C123ABC',credentialRef:'credential_fixture'};
test('Slack posts bounded text to a fixed route and returns message identity',async()=>{
 assert.deepEqual(await invoke('slack.postMessage',{text:'Fixture'},slack,async request=>{
  assert.deepEqual(request,{url:'https://slack.com/api/chat.postMessage',method:'POST',body:{channel:'C123ABC',text:'Fixture'}});
  return {status:200,body:{ok:true,channel:'C123ABC',ts:'123456.000001',message:{text:'not retained'}}};
 }),{channel:'C123ABC',ts:'123456.000001'});
});
test('Slack HTTP 200 rejection is a safe failure, never success',async()=>{
 await assert.rejects(invoke('slack.postMessage',{text:'Fixture'},slack,async()=>({status:200,body:{ok:false,error:'sensitive-provider-detail'}})),error=>{
  assert.equal((error as {code:string}).code,'ProviderRejected');assert.ok(!String(error).includes('sensitive-provider-detail'));return true;
 });
 for(const body of [{},{ok:'true'},{ok:true,channel:'C123ABC',ts:123}]) await assert.rejects(invoke('slack.postMessage',{text:'Fixture'},slack,async()=>({status:200,body})),{code:'InvalidOutput'});
});
test('Slack rejects invalid configuration and input before send',async()=>{
 for(const config of [{...slack,channel:'#general'},{channel:'C123ABC'}]) await assert.rejects(invoke('slack.postMessage',{text:'Fixture'},config,async()=>assert.fail('must not send')),{code:'InvalidInput'});
 for(const input of [{text:''},{text:'x'.repeat(3001)},{text:'hello',channel:'COTHER'}]) await assert.rejects(invoke('slack.postMessage',input,slack,async()=>assert.fail('must not send')),{code:'InvalidInput'});
 const controller=new AbortController();controller.abort();
 await assert.rejects(invoke('slack.postMessage',{text:'Fixture'},slack,async()=>assert.fail('cancelled'),controller.signal),{name:'AbortError'});
});

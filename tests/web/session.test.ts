import assert from 'node:assert/strict';
import { test } from 'node:test';
import { request, setCSRFToken, APIError, sessionExpiredEvent } from '../../web/src/api.ts';

test('cookie API sends CSRF only on writes and rejects external destinations', async t => {
  const calls: RequestInit[]=[];
  t.mock.method(globalThis,'fetch',async (_path: string,init: RequestInit)=>{calls.push(init);return new Response('{}',{status:200});});
  setCSRFToken('fixture-csrf');
  await request('/v1/workspaces/fixture/workflows','POST',{});
  await request('/v1/workspaces/fixture/workflows');
  assert.equal(calls[0].credentials,'same-origin');
  assert.equal(new Headers(calls[0].headers).get('X-CSRF-Token'),'fixture-csrf');
  assert.equal(new Headers(calls[1].headers).has('X-CSRF-Token'),false);
  for(const path of ['https://other.example/v1/','//other.example/v1/','/v1/\\other.example']) await assert.rejects(request(path,'POST'),/local/);
  assert.equal(calls.length,2);
  setCSRFToken('');
});
test('unauthorized requests clear CSRF state and notify the session boundary',async t=>{
  const previous=Object.getOwnPropertyDescriptor(globalThis,'window');
  const events=new EventTarget();
  Object.defineProperty(globalThis,'window',{value:events,configurable:true});
  t.after(()=>{if(previous)Object.defineProperty(globalThis,'window',previous);else delete globalThis.window;});
  let expired=0;events.addEventListener(sessionExpiredEvent,()=>expired++);
  let status=401;let headers:Headers;
  t.mock.method(globalThis,'fetch',async (_path:string,init:RequestInit)=>{headers=new Headers(init.headers);return new Response('{}',{status});});
  setCSRFToken('fixture-csrf');
  await assert.rejects(request('/v1/identity'),e=>e instanceof APIError && e.status===401);
  assert.equal(expired,1);
  status=200;
  await request('/v1/auth/logout','POST');
  assert.equal(headers!.has('X-CSRF-Token'),false);
});

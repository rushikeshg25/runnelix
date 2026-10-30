import assert from 'node:assert/strict';
import { test } from 'node:test';
import { portableImportBody, portableExportText } from '../../web/src/portable.ts';

test('import retains duplicate keys for server rejection', () => {
  const source='{"name":"first","name":"second","graph":{"version":1,"version":2}}';
  assert.equal(portableImportBody(source,{}),`{"bundle":${source},"bindings":{}}`);
  assert.throws(()=>portableImportBody('{} {}',{}));
});
test('exported large valid graph fits the file and import limits', () => {
  const start={id:'start',type:'trigger.manual',version:1,config:{}};
  const nodes=Array.from({length:99},(_,i)=>({id:'node_'+String(i).padStart(59,'0'),type:'http.get',version:1,config:{url:'https://example.com/'+ 'a'.repeat(2028),credentialRef:`slot_${i+1}`}}));
  const graph={formatVersion:1,nodes:[start,...nodes],edges:nodes.map((node,i)=>({from:{node:i===0?'start':nodes[i-1]!.id,port:'out'},to:{node:node.id,port:'in'}}))};
  const bundle={formatVersion:1,name:'Large export',graph,credentialSlots:nodes.map((_,i)=>({slot:`slot_${i+1}`,kind:'bearer',destination:'example.com'}))};
  assert.ok(Buffer.byteLength(JSON.stringify(bundle,null,2))>256*1024,'fixture must expose previous pretty-print overflow');
  const source=portableExportText(bundle);
  assert.ok(Buffer.byteLength(source)<=256*1024);
  const bindings=Object.fromEntries(bundle.credentialSlots.map(s=>[s.slot,'credential_'+ 'a'.repeat(36)]));
  assert.deepEqual(JSON.parse(portableImportBody(source,bindings)).bundle,bundle);
});
test('import bounds UTF-8 bytes including credential bindings', () => {
  assert.throws(()=>portableImportBody(JSON.stringify({text:'é'.repeat(140000)}),{}),/256 KiB/);
  assert.throws(()=>portableImportBody('{}',{slot:'x'.repeat(256*1024)}),/256 KiB/);
});

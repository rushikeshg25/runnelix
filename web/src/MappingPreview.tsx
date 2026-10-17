import { useState } from 'react';
import { request } from './api';
export function MappingPreview({ expression }: { expression: string }) {
 const [input, setInput] = useState('{"message":"sample"}'); const [result, setResult] = useState('');
 return <details className="mapping-preview"><summary>Preview expression</summary><label>Sample input<textarea rows={3} value={input} onChange={e => setInput(e.target.value)} /></label><button onClick={() => { void request<{ value: unknown }>('/v1/mapping-preview', 'POST', { expression, input: (() => { try { return JSON.parse(input); } catch { return undefined; } })() }).then(v => setResult(JSON.stringify(v.value, null, 2))).catch(() => setResult('Invalid sample or expression. Check the JSON and CEL expression.')); }}>Evaluate sample</button><pre aria-live="polite">{result}</pre></details>;
}

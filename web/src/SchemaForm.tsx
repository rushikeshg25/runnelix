import { JsonField } from "./JsonField";
import type { Descriptor, GraphNode } from "./api";
export function SchemaForm({
  descriptor,
  node,
  change,
}: {
  descriptor: Descriptor;
  node: GraphNode;
  change: (config: Record<string, unknown>) => void;
}) {
  const fields = Object.entries(descriptor.configSchema.properties ?? {});
  if (!fields.length)
    return <p className="muted">This node needs no configuration.</p>;
  return (
    <div className="fields">
      {fields.map(([name, schema]) => {
        const required = descriptor.configSchema.required?.includes(name);
        const value = node.config[name];
        const update = (raw: string) => {
          const next = { ...node.config };
          if (!raw && !required) delete next[name];
          else
            next[name] =
              schema.type === "integer" || schema.type === "number"
                ? Number(raw)
                : raw;
          change(next);
        };
        return (
          <label key={name}>
            {schema.title ?? name}
            {required ? " *" : " (optional)"}
            {schema.type === "object" ? (
              <JsonField
                key={`${node.id}/${name}`}
                value={value}
                change={(value) => change({ ...node.config, [name]: value })}
              />
            ) : schema.enum ? (
              <select
                value={String(value ?? "")}
                onChange={(e) => update(e.target.value)}
              >
                <option value="">Choose…</option>
                {schema.enum.map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            ) : name.toLowerCase().endsWith("expression") ? (
              <textarea
                spellCheck={false}
                rows={4}
                value={String(value ?? "")}
                onChange={(e) => update(e.target.value)}
                maxLength={schema.maxLength}
              />
            ) : (
              <input
                type={
                  schema.type === "integer" || schema.type === "number"
                    ? "number"
                    : "text"
                }
                value={String(value ?? "")}
                onChange={(e) => update(e.target.value)}
                min={schema.minimum}
                max={schema.maximum}
                maxLength={schema.maxLength}
                required={required}
              />
            )}
            {name === "credentialRef" && (
              <small>
                Use a granted credential ID. Never paste a secret here.
              </small>
            )}
            {name === "url" && (
              <small>
                The host must be approved by your local administrator.
              </small>
            )}
            {name.toLowerCase().endsWith("expression") && (
              <small>
                CEL expression. Read the previous node’s value as{" "}
                <code>input</code>.
              </small>
            )}
          </label>
        );
      })}
    </div>
  );
}

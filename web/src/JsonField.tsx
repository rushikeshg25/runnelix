import { useState } from "react";

// Keep invalid text visible and in the draft so validation cannot publish an older value.
export function JsonField({
  value,
  change,
}: {
  value: unknown;
  change: (value: unknown) => void;
}) {
  const [text, setText] = useState(JSON.stringify(value ?? {}, null, 2));
  const [error, setError] = useState("");
  return (
    <>
      <textarea
        rows={5}
        spellCheck={false}
        value={text}
        aria-invalid={!!error}
        onChange={(e) => {
          const raw = e.target.value;
          setText(raw);
          try {
            const parsed: unknown = JSON.parse(raw);
            if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
              throw new Error("Enter a JSON object.");
            change(parsed);
            setError("");
          } catch {
            change(raw);
            setError(
              "Enter a valid JSON object. The draft cannot publish until this is fixed.",
            );
          }
        }}
      />
      {error && <small role="alert">{error}</small>}
    </>
  );
}

import type { Scale } from "../types";
import { type Input, range, stepOf } from "./scale";

export function ScaleInput({ scale, input, onChange }: { scale: Scale; input: Input; onChange: (i: Input) => void }) {
  switch (input.kind) {
    case "decision":
      return (
        <div className="choices" role="group" aria-label="Decision">
          {(scale.options ?? []).map((o) => (
            <button key={o} type="button" aria-pressed={input.choice === o} onClick={() => onChange({ kind: "decision", choice: o })}>
              {o}
            </button>
          ))}
        </div>
      );
    case "label":
      return (
        <label>
          Label
          <select value={input.choice ?? ""} onChange={(e) => onChange({ kind: "label", choice: e.target.value || null })}>
            <option value="">Choose…</option>
            {(scale.labels ?? []).map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
        </label>
      );
    case "score": {
      const { lo, hi } = range(scale);
      return (
        <label>
          Score
          <input type="range" min={lo} max={hi} step={stepOf(scale)} value={input.x} onChange={(e) => onChange({ kind: "score", x: Number(e.target.value) })} />
          <output>{input.x.toFixed(2)}</output>
        </label>
      );
    }
    case "text":
      return (
        <label>
          Verdict
          <textarea value={input.text} onChange={(e) => onChange({ kind: "text", text: e.target.value })} />
        </label>
      );
  }
}

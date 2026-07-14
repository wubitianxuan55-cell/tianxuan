import { useEffect, useState } from "react";

export function StepLimitControl({
  value,
  presets,
  busy,
  onChange,
}: {
  value: number;
  presets: number[];
  busy: boolean;
  onChange: (value: number) => void;
}) {
  const normalized = Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
  const presetSet = new Set(presets.map((p) => (Number.isFinite(p) && p > 0 ? Math.trunc(p) : 0)));
  const [custom, setCustom] = useState(String(normalized));
  useEffect(() => setCustom(String(normalized)), [normalized]);
  const isCustom = !presetSet.has(normalized);
  const commitCustom = () => {
    const next = Number.isFinite(Number(custom)) && Number(custom) > 0 ? Math.trunc(Number(custom)) : 0;
    setCustom(String(next));
    if (next !== normalized) onChange(next);
  };
  return (
    <div className="flex items-center gap-2">
      <div className="flex rounded-md border border-border-soft overflow-hidden text-[12px]">
        {presets.map((preset) => {
          const n = Number.isFinite(preset) && preset > 0 ? Math.trunc(preset) : 0;
          return (
            <button
              key={n}
              type="button"
              className={`px-3 py-1 border-0 bg-transparent cursor-pointer transition-colors ${
                normalized === n ? "bg-accent text-white" : "text-fg-dim hover:bg-bg-soft"
              }`}
              disabled={busy}
              onClick={() => n !== normalized && onChange(n)}
            >
              {n === 0 ? "∞" : String(n)}
            </button>
          );
        })}
        <button
          type="button"
          className={`px-3 py-1 border-0 bg-transparent cursor-pointer transition-colors ${
            isCustom ? "bg-accent text-white" : "text-fg-dim hover:bg-bg-soft"
          }`}
          disabled={busy}
          onClick={() => { if (!isCustom) setCustom(String(normalized || 12)); }}
        >
          自定义
        </button>
      </div>
      <input
        className="w-16 bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2 py-1 outline-none placeholder:text-fg-faint focus:border-accent"
        value={custom}
        disabled={busy}
        inputMode="numeric"
        aria-label="自定义步数"
        onChange={(e) => setCustom(e.target.value.replace(/\D/g, ""))}
        onBlur={commitCustom}
        onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
      />
    </div>
  );
}

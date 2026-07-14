// SettingsPageShell wraps a settings page with a title and optional description.
export function SettingsPageShell({ title, desc, children }: { title: React.ReactNode; desc?: string; children: React.ReactNode }) {
  return (
    <div className="settings-page">
      <h2 className="text-[17px] font-bold text-fg pb-3 mb-4 border-b border-border-soft/40">{title}</h2>
      {desc && <p className="text-[13px] text-fg-dim mb-5">{desc}</p>}
      {children}
    </div>
  );
}

// SettingsSection groups related fields under a section header.
export function SettingsSection({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="mb-5">
      <div className="bg-bg-soft border border-border-soft rounded-xl p-4 shadow-[0_1px_3px_rgba(0,0,0,0.2)]">
        <h3 className="text-[13px] font-semibold text-fg mb-3">{title}</h3>
        <div className="flex flex-col gap-3">{children}</div>
      </div>
    </section>
  );
}

// SettingsField renders a single labeled control row. Control is the right-aligned widget.
export function SettingsField({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-2 border-b border-border-soft/50 last:border-b-0">
      <div className="flex flex-col min-w-0">
        <span className="text-[13px] text-fg">{label}</span>
        {hint && <span className="text-[11px] text-fg-faint mt-0.5">{hint}</span>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

// SegmentedButton renders a segmented button group for mutually exclusive choices.
export function SegmentedButton<T extends string>({ options, value, onChange }: {
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div className="flex rounded-md border border-border overflow-hidden text-[12px]">
      {options.map((opt) => (
        <button
          key={opt.value}
          className={`px-3 py-1 border-0 bg-transparent cursor-pointer transition-colors ${
            value === opt.value ? "bg-accent/22 text-accent font-semibold ring-1 ring-accent/40" : "text-fg-dim hover:bg-bg hover:text-fg"
          }`}
          onClick={() => onChange(opt.value)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

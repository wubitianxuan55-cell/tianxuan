import type { SVGProps, ReactNode } from "react";

// 执行流程可视化组件 —— 用 SVG 图标展示 Planner→Executor 的流程步骤。
// 颜色语义：default=灰色 / success=绿色 / warning=琥珀色 / danger=红色
// (Design adopted from DeepSeek-Reasonix-V1.12)

type IconProps = SVGProps<SVGSVGElement> & { size?: number };
export type ProcessTone = "default" | "success" | "warning" | "danger" | "accent" | "violet";
export type ProcessState = "running" | "done" | "failed" | "waiting" | "stopped";

const TONE_COLORS: Record<ProcessTone, string> = {
  default: "var(--ds-fg-faint)",
  success: "var(--ds-ok)",
  warning: "var(--ds-warn)",
  danger: "var(--ds-err)",
  accent: "var(--ds-accent)",
  violet: "#8b5cf6",
};

const STATE_COLORS: Record<ProcessState, string> = {
  running: "var(--ds-accent)",
  done: "var(--ds-ok)",
  failed: "var(--ds-err)",
  waiting: "var(--ds-fg-faint)",
  stopped: "var(--ds-warn)",
};

function ProcessIcon({ size = 14, tone = "default", children, ...rest }: IconProps & { tone?: ProcessTone; children: ReactNode }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={TONE_COLORS[tone]}
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...rest}
    >
      {children}
    </svg>
  );
}

export function ProcessChevronIcon(props: IconProps & { tone?: ProcessTone }) {
  return <ProcessIcon {...props}><path d="m6 9 6 6 6-6" /></ProcessIcon>;
}

export function ProcessCheckIcon(props: IconProps & { tone?: ProcessTone }) {
  return <ProcessIcon {...props}><path d="m5 12 5 5L20 7" /></ProcessIcon>;
}

export function ProcessXIcon(props: IconProps & { tone?: ProcessTone }) {
  return <ProcessIcon {...props}><path d="M6 6l12 12M18 6 6 18" /></ProcessIcon>;
}

export function ProcessBrainIcon(props: IconProps & { tone?: ProcessTone }) {
  return (
    <ProcessIcon {...props}>
      <path d="M9 4a3 3 0 0 0-3 3v0a3 3 0 0 0-2 5 3 3 0 0 0 2 5 3 3 0 0 0 3 3h0a3 3 0 0 0 3-3V4" />
      <path d="M15 4a3 3 0 0 1 3 3 3 3 0 0 1 2 5 3 3 0 0 1-2 5 3 3 0 0 1-3 3" />
    </ProcessIcon>
  );
}

export function ProcessToolIcon(props: IconProps & { tone?: ProcessTone }) {
  return (
    <ProcessIcon {...props}>
      <path d="M14 7a4 4 0 1 0 4 4l3 3-3 3-3-3a4 4 0 0 1-4-4l-3-3-3 3 3 3a4 4 0 0 0 6 0" />
    </ProcessIcon>
  );
}

export function ProcessInfoIcon(props: IconProps & { tone?: ProcessTone }) {
  return (
    <ProcessIcon {...props}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 16v-4" />
      <path d="M12 8h.01" />
    </ProcessIcon>
  );
}

export function ProcessPhaseIcon(props: IconProps & { tone?: ProcessTone }) {
  return (
    <ProcessIcon {...props}>
      <path d="M4 7h9" />
      <path d="M4 12h7" />
      <path d="M4 17h5" />
      <path d="M16 7h4" />
      <path d="M16 12h4" />
      <path d="M16 17h4" />
    </ProcessIcon>
  );
}

export function ProcessCompactIcon(props: IconProps & { tone?: ProcessTone }) {
  return (
    <ProcessIcon {...props}>
      <path d="M4 6h16M4 12h16M4 18h16" />
    </ProcessIcon>
  );
}

/** 根据 ProcessState 返回对应的 tone。 */
export function processStateToTone(state: ProcessState): ProcessTone {
  switch (state) {
    case "done": return "success";
    case "failed": return "danger";
    case "running": return "accent";
    case "stopped": return "warning";
    default: return "default";
  }
}

/** 获取 ProcessState 对应的 CSS 颜色值。 */
export function stateColor(state: ProcessState): string {
  return STATE_COLORS[state] ?? STATE_COLORS.waiting;
}

/** 获取 ProcessTone 对应的 CSS 颜色值。 */
export function toneColor(tone: ProcessTone): string {
  return TONE_COLORS[tone] ?? TONE_COLORS.default;
}

// ─── 趋势图通用渲染组件 ──────────────────────────────────
// 从 V10.45.0 之前的 MiniAreaChart 恢复，提供统一的 SVG 折线/面积图渲染
// 支持：Y 轴刻度标签、面积填充、折线、数据点、X 轴标签

export interface TrendPoint {
  x: number;
  y: number;
  label: string;
}

export function TrendChart({
  title,
  W, H, padL, padR, padT, padB,
  points, yTicks, color, xLabels, fillOpacity,
}: {
  title?: string;
  W: number; H: number; padL: number; padR: number; padT: number; padB: number;
  points: TrendPoint[];
  yTicks: [number, string][];
  color: string;
  xLabels: { at: number; text: string }[];
  fillOpacity?: number;
}) {
  const plotH = H - padT - padB;
  const path = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
  const last = points[points.length - 1];
  const first = points[0];
  const areaPath = fillOpacity ? `${path} L${last.x.toFixed(1)},${padT + plotH} L${first.x.toFixed(1)},${padT + plotH} Z` : "";
  return (
    <div className="py-3 border-b border-border-soft">
      {title && (
        <div className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mb-2">{title}</div>
      )}
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto">
        {yTicks.map(([_val, label], i) => {
          const y = padT + plotH - (i / (yTicks.length - 1)) * plotH;
          return (
            <g key={`y${i}`}>
              <line x1={padL} y1={y} x2={W - padR} y2={y} stroke="var(--border-soft)" strokeWidth={0.5} />
              <text x={padL - 4} y={y + 3} fontSize={9} fill="var(--fg-faint)" textAnchor="end">{label}</text>
            </g>
          );
        })}
        {fillOpacity && areaPath && (
          <path d={areaPath} fill={color} opacity={fillOpacity} />
        )}
        <path d={path} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" />
        {points.map((p, i) => (
          <circle key={i} cx={p.x} cy={p.y} r={2} fill={color}>
            <title>{p.label}</title>
          </circle>
        ))}
        {xLabels.map((xl, i) => (
          <text key={i} x={xl.at} y={H - 3} fontSize={9} fill="var(--fg-faint)" textAnchor="middle">{xl.text}</text>
        ))}
      </svg>
    </div>
  );
}

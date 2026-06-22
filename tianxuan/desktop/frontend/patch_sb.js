const fs = require('fs');

// ── StatusBar.tsx ──
let sb = fs.readFileSync('tianxuan/desktop/frontend/src/components/StatusBar.tsx', 'utf8');

// Add agentMode to props destructure
sb = sb.replace(
  'running: boolean;\n  mode: Mode;',
  'running: boolean;\n  mode: Mode;\n  agentMode?: string;'
);

// Add agentMode badge before the plan/yolo badges
sb = sb.replace(
  '{/* 模式 */}\n        {mode === "yolo"',
  '{/* Agent 模式 */}\n        {agentMode && agentMode !== "" && (\n          <span className={`${fontSize} px-1.5 py-px rounded border font-medium ${\n            agentMode === "explore" ? "text-info bg-info/10 border-info/20" :\n            agentMode === "orchestrate" ? "text-accent bg-accent-soft border-accent/30" :\n            "text-ok bg-ok/10 border-ok/20"\n          }`}>\n            {agentMode === "explore" ? "EXPLORE" : agentMode === "develop" ? "DEVELOP" : "ORCH"}\n          </span>\n        )}\n\n        {/* 模式 */}\n        {mode === "yolo"'
);

fs.writeFileSync('tianxuan/desktop/frontend/src/components/StatusBar.tsx', sb);
console.log('StatusBar.tsx done');

const fs = require('fs');

// ── Composer.tsx ──
let cmp = fs.readFileSync('tianxuan/desktop/frontend/src/components/Composer.tsx', 'utf8');

// Add agentMode/onSetAgentMode to props destructure
cmp = cmp.replace(
  'running, mode, cwd, onSend, onCancel, onCycleMode, onPickFolder, disabled,',
  'running, mode, cwd, onSend, onCancel, onCycleMode, agentMode, onSetAgentMode, onPickFolder, disabled,'
);

// Add to type interface
cmp = cmp.replace(
  'onCancel: () => string | undefined; onCycleMode: () => void;',
  'onCancel: () => string | undefined; onCycleMode: () => void; agentMode?: string; onSetAgentMode?: (m: string) => void;'
);

// Add agentMode buttons after normal/plan/yolo buttons
const oldSection = '          {/* 快捷提示 */}\n          <span className="ml-auto text-fg-faint/40 text-[10px] select-none hidden sm:inline-flex items-center gap-1.5">';
const newSection = '          {/* Agent 模式 */}\n          <div className="flex gap-[3px] ml-2">\n            {(["explore", "develop", "orchestrate"] as string[]).map((am) => (\n              <button key={am} type="button"\n                className={`flex items-center gap-1 px-1.5 py-0.5 border rounded text-[10px] cursor-pointer transition-[color,background,border] duration-[var(--dur-fast)] ${\n                  agentMode === am ? "text-accent bg-accent-soft border-accent/30" : "text-fg-faint border-border-soft hover:text-fg hover:bg-bg-soft"\n                }`}\n                onClick={() => { if (agentMode !== am && onSetAgentMode) onSetAgentMode(am); }}\n                title={am === "explore" ? "探索模式" : am === "develop" ? "开发模式" : "编排模式"}\n              >\n                <span className={`w-1.5 h-1.5 rounded-full ${am === "explore" ? "bg-info" : am === "develop" ? "bg-ok" : "bg-accent"}`} />\n                {am === "explore" ? "探索" : am === "develop" ? "开发" : "编排"}\n              </button>\n            ))}\n          </div>\n\n' + oldSection;

cmp = cmp.replace(oldSection, newSection);

fs.writeFileSync('tianxuan/desktop/frontend/src/components/Composer.tsx', cmp);
console.log('Composer.tsx done');

import { readFileSync, writeFileSync } from 'fs';

const zh = readFileSync('src/locales/zh.ts', 'utf8');
const s2 = zh.replace(
  '  "composer.modeYolo": "YOLO",\r\n  "composer.modeHint": "shift+tab",',
  '  "composer.modeYolo": "YOLO",\r\n  "composer.modeExplore": "\u63a2\u7d22",\r\n  "composer.modeDevelop": "\u5f00\u53d1",\r\n  "composer.modeOrchestrate": "\u7f16\u6392",\r\n  "composer.agentModeTitle": "\u5207\u6362 Agent \u6a21\u5f0f\uff08\u63a2\u7d22 / \u5f00\u53d1 / \u7f16\u6392\uff09",\r\n  "composer.modeHint": "shift+tab",'
);
writeFileSync('src/locales/zh.ts', s2);

const en = readFileSync('src/locales/en.ts', 'utf8');
const e2 = en.replace(
  '  "composer.modeYolo": "YOLO",\r\n  "composer.modeHint": "shift+tab",',
  '  "composer.modeYolo": "YOLO",\r\n  "composer.modeExplore": "explore",\r\n  "composer.modeDevelop": "develop",\r\n  "composer.modeOrchestrate": "orchestrate",\r\n  "composer.agentModeTitle": "Switch agent mode (explore / develop / orchestrate)",\r\n  "composer.modeHint": "shift+tab",'
);
writeFileSync('src/locales/en.ts', e2);
console.log('i18n done');

// find_unused_css.ts — 检测 styles.css 中未被 TSX 引用的类名
// 用法: npx tsx find_unused_css.ts

import * as fs from "fs";
import * as path from "path";

// Run from the frontend directory: npx tsx find_unused_css.ts
const root = process.cwd();
const cssPath = path.join(root, "src", "styles.css");
const srcDir = path.join(root, "src");

const css = fs.readFileSync(cssPath, "utf-8");

// 提取所有 CSS 类选择器（以 . 开头，后跟字母/-/_）
const classRe = /\.([a-zA-Z_][\w-]*(?:::[a-zA-Z_][\w-]*)?)\b/g;
const cssClasses = new Set<string>();
let m: RegExpExecArray | null;
while ((m = classRe.exec(css)) !== null) {
  cssClasses.add(m[1]);
}

// 收集所有 TSX 文件内容
function collectTsx(dir: string): string[] {
  const files: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory() && entry.name !== "node_modules" && entry.name !== "wailsjs") {
      files.push(...collectTsx(p));
    } else if (entry.isFile() && /\.(tsx|ts|css)$/.test(entry.name)) {
      files.push(p);
    }
  }
  return files;
}

const tsxFiles = collectTsx(srcDir);
let tsxContent = "";
for (const f of tsxFiles) {
  tsxContent += fs.readFileSync(f, "utf-8") + "\n";
}

// 检查哪些 CSS 类名未在 TSX/CSS 源码中出现
const unused: string[] = [];
for (const cls of cssClasses) {
  // className 中的引用可能是 "cls" 或 'cls' 或 `cls`
  if (!tsxContent.includes(`"${cls}"`) && !tsxContent.includes(`'${cls}'`) && !tsxContent.includes(`\`${cls}\``)) {
    // 也检查 className={...} 动态引用
    if (!new RegExp(`\\b${cls.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`).test(tsxContent)) {
      unused.push(cls);
    }
  }
}

// 输出
unused.sort();
console.log(`CSS classes in styles.css: ${cssClasses.size}`);
console.log(`Unused CSS classes: ${unused.length}`);
console.log("");
for (const cls of unused) {
  // 找到该类的 CSS 位置
  const idx = css.indexOf(`.${cls}`);
  const lineNum = css.substring(0, idx).split("\n").length;
  console.log(`  .${cls}  (line ${lineNum})`);
}

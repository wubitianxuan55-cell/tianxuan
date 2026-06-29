// Pure language resolution — no highlight.js dependency. Non-lazy consumers (e.g.
// ToolCard) guess a language from a path without pulling the highlighter into the
// main bundle; highlight.js stays behind the lazy editor seam. highlight.ts
// imports ALIASES from here and adds the hljs-backed validation.

export const ALIASES: Record<string, string> = {
  // JavaScript family
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  // Shell
  sh: "bash",
  shell: "bash",
  zsh: "bash",
  bash: "bash",
  // Python
  py: "python",
  python3: "python",
  // Rust
  rs: "rust",
  // YAML
  yml: "yaml",
  // XML/HTML
  html: "xml",
  svg: "xml",
  // Markdown
  md: "markdown",
  markdown: "markdown",
  // C/C++
  c: "c",
  h: "c",
  cc: "cpp",
  cpp: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  "c++": "cpp",
  // C#
  cs: "csharp",
  "c#": "csharp",
  // Java
  java: "java",
  // SQL
  sql: "sql",
  // Diff
  diff: "diff",
  patch: "diff",
  // Docker
  dockerfile: "dockerfile",
  docker: "dockerfile",
  // Config
  toml: "toml",
  ini: "ini",
  cfg: "ini",
  conf: "ini",
  properties: "ini",
  // Make
  makefile: "makefile",
  mk: "makefile",
  make: "makefile",
  // Nginx
  nginx: "nginx",
  // Plain text — explicitly map to empty so we don't try to highlight
  text: "",
  plaintext: "",
  txt: "",
};

const EXT: Record<string, string> = {
  go: "go",
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  json: "json",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  py: "python",
  rs: "rust",
  html: "xml",
  xml: "xml",
  svg: "xml",
  css: "css",
  yaml: "yaml",
  yml: "yaml",
  md: "markdown",
  c: "c",
  h: "c",
  cc: "cpp",
  cpp: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  cs: "csharp",
  java: "java",
  sql: "sql",
  toml: "toml",
  ini: "ini",
  cfg: "ini",
  conf: "ini",
  diff: "diff",
  patch: "diff",
};

// extToLang infers a language name from a file path's extension (for tool diffs).
export function extToLang(path: string): string {
  const dot = path.lastIndexOf(".");
  if (dot < 0) return "";
  return EXT[path.slice(dot + 1).toLowerCase()] ?? "";
}

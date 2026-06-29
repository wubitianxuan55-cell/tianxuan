// Syntax highlighting via highlight.js core with a hand-picked language set
// (registering only what a coding agent surfaces keeps the bundle lean). This is
// the engine behind the editor seam's HljsCode / HljsDiff; token colors are
// themed in styles.css (.hljs-*) to match the app palette rather than a stock CSS.
//
// V10.12: added LRU cache (djb2 hash, 200 entries) + 8 more languages.

import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import c from "highlight.js/lib/languages/c";
import cpp from "highlight.js/lib/languages/cpp";
import csharp from "highlight.js/lib/languages/csharp";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import dockerfile from "highlight.js/lib/languages/dockerfile";
import go from "highlight.js/lib/languages/go";
import ini from "highlight.js/lib/languages/ini";
import java from "highlight.js/lib/languages/java";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import makefile from "highlight.js/lib/languages/makefile";
import markdown from "highlight.js/lib/languages/markdown";
import nginx from "highlight.js/lib/languages/nginx";
import python from "highlight.js/lib/languages/python";
import rust from "highlight.js/lib/languages/rust";
import sql from "highlight.js/lib/languages/sql";
import toml from "highlight.js/lib/languages/ini"; // toml uses ini grammar
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";

import { ALIASES } from "./lang";

hljs.registerLanguage("bash", bash);
hljs.registerLanguage("c", c);
hljs.registerLanguage("cpp", cpp);
hljs.registerLanguage("csharp", csharp);
hljs.registerLanguage("css", css);
hljs.registerLanguage("diff", diff);
hljs.registerLanguage("dockerfile", dockerfile);
hljs.registerLanguage("go", go);
hljs.registerLanguage("ini", ini);
hljs.registerLanguage("java", java);
hljs.registerLanguage("javascript", javascript);
hljs.registerLanguage("json", json);
hljs.registerLanguage("makefile", makefile);
hljs.registerLanguage("markdown", markdown);
hljs.registerLanguage("nginx", nginx);
hljs.registerLanguage("python", python);
hljs.registerLanguage("rust", rust);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("toml", toml); // ini-based
hljs.registerLanguage("typescript", typescript);
hljs.registerLanguage("xml", xml);
hljs.registerLanguage("yaml", yaml);

// ── LRU cache ──────────────────────────────────────────────────────────────

const CACHE_CAP = 200;

interface CacheEntry {
  key: string;
  code: string;
  html: string;
}

class HljsCache {
  private map = new Map<string, CacheEntry>();
  private head: CacheEntry = null as any;
  private tail: CacheEntry = null as any;

  get(key: string, code: string): string | undefined {
    const e = this.map.get(key);
    if (!e) return undefined;
    // 防哈希碰撞：二次校验原始代码
    if (e.code !== code) {
      this.map.delete(key);
      return undefined;
    }
    this.moveToHead(e);
    return e.html;
  }

  set(key: string, code: string, html: string) {
    // 如果已存在则更新
    const existing = this.map.get(key);
    if (existing) {
      existing.code = code;
      existing.html = html;
      this.moveToHead(existing);
      return;
    }
    // 淘汰最久未使用
    if (this.map.size >= CACHE_CAP && this.tail) {
      this.map.delete(this.tail.key);
      this.removeNode(this.tail);
    }
    const e: CacheEntry = { key, code, html };
    this.map.set(key, e);
    this.addToHead(e);
  }

  private moveToHead(e: CacheEntry) {
    if (this.head === e) return;
    this.removeNode(e);
    this.addToHead(e);
  }

  private addToHead(e: CacheEntry) {
    (e as any).prev = null;
    (e as any).next = this.head;
    if (this.head) (this.head as any).prev = e;
    this.head = e;
    if (!this.tail) this.tail = e;
  }

  private removeNode(e: CacheEntry) {
    const prev = (e as any).prev;
    const next = (e as any).next;
    if (prev) (prev as any).next = next;
    if (next) (next as any).prev = prev;
    if (this.head === e) this.head = next;
    if (this.tail === e) this.tail = prev;
  }
}

const cache = new HljsCache();

// djb2 hash — fast, good distribution for short strings
function hashKey(lang: string, code: string): string {
  let h = 5381;
  for (let i = 0; i < lang.length; i++) h = ((h << 5) + h) ^ lang.charCodeAt(i);
  h = ((h << 5) + h) ^ 0; // separator
  for (let i = 0; i < code.length; i++) h = ((h << 5) + h) ^ code.charCodeAt(i);
  return String(h >>> 0);
}

// ── public API ─────────────────────────────────────────────────────────────

function escapeHtml(s: string): string {
  return s.replace(/[&<>]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"));
}

// resolveLang maps a markdown fence tag or guessed name to a registered language,
// or "" when we can't highlight it (caller renders escaped plain text).
export function resolveLang(lang?: string): string {
  if (!lang) return "";
  const l = lang.toLowerCase();
  const resolved = ALIASES[l] ?? l;
  return hljs.getLanguage(resolved) ? resolved : "";
}

// highlightToHtml returns highlighted HTML (token <span>s) for the given code, or
// escaped plain text when the language is unknown. ignoreIllegals so partial /
// streaming snippets never throw. Results are cached via LRU.
export function highlightToHtml(code: string, lang?: string): string {
  const resolved = resolveLang(lang);
  if (!resolved) return escapeHtml(code);

  const key = hashKey(resolved, code);
  const cached = cache.get(key, code);
  if (cached !== undefined) return cached;

  try {
    const html = hljs.highlight(code, { language: resolved, ignoreIllegals: true }).value;
    cache.set(key, code, html);
    return html;
  } catch {
    return escapeHtml(code);
  }
}

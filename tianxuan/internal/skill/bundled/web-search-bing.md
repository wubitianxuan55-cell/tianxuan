---
name: web-search-bing
description: 用 Bing 搜索替代 DuckDuckGo（国内不可用）。使用 web_fetch 抓取 Bing 搜索结果页面并提取摘要。以中文搜索为佳。
---

# Web Search via Bing

DuckDuckGo API 在中国网络环境下不可用。此 skill 用 `web_fetch` + Bing 作为替代。

## 使用方法

收到搜索任务后：
1. 构造 URL: `https://www.bing.com/search?q=<URL编码的查询词>`
2. 用 `web_fetch` 抓取该 URL
3. 从返回的 HTML 中提取搜索结果标题、URL 和摘要
4. 若需要详情，再用 `web_fetch` 抓取具体页面

## 搜索技巧

- 中文查询直接使用中文关键词（Bing 国内版对中文友好）
- 英文技术查询建议加 `site:github.com` 或具体域名限定
- 查询尽量具体，避免宽泛词
- `count=N` 参数可追加但 Bing 不总是遵守

## 示例

查询 "Cursor IDE session management UX":
```
web_fetch url="https://www.bing.com/search?q=Cursor+IDE+session+management+UX"
```

查询中文内容:
```
web_fetch url="https://www.bing.com/search?q=AI编程助手+会话管理+设计"
```

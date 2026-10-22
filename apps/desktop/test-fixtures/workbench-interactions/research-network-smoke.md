# 联网取材核验 2026-10-22

来源：
https://example.com/
https://httpbin.org/json

核验：
1. example.com：HTTP 200，标题 "Example Domain"；仅用于文档示例，无需授权，非服务，不宜用于测试或监控。
2. httpbin.org/json：HTTP 200，application/json；slideshow.title "Sample Slide Show"，author "Yours Truly"，slides 2 项。
3. httpbin 首页（v0.9.2）："A simple HTTP Request & Response Service."，供 HTTP 客户端调试与接口测试。

HTML 与 JSON：前者为 text/html 静态页，人类可读、无结构字段；后者为 JSON，可程序解析，含嵌套对象与数组。

核验方式：两源均由子智能体 WebFetch 成功取得。

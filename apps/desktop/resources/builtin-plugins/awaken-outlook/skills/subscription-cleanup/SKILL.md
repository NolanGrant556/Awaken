---
name: subscription-cleanup
description: 识别 Outlook 新闻简报和重复订阅邮件，检查标准退订方式，并在用户确认后执行一键退订、mailto 退订、分类、移动或删除。用户要求退订或清理营销邮件时使用。
metadata:
  display_name: 订阅邮件清理
---

# Outlook 订阅邮件清理

## 候选识别

1. 用 `search_messages` 按发件人、主题或新闻简报关键词建立数量受限的候选；只按近期/未读浏览时才使用 `list_messages`。
2. 区分真正订阅、交易凭证、安全提醒、业务告警和人工邮件。自动发送不等于低价值。
3. 对每个退订候选调用 `get_unsubscribe_info`，确认它属于：
   - `one_click`：邮件声明 RFC 8058 HTTPS 一键退订；
   - `mailto`：需要发送退订邮件；
   - `external_page`：只能打开网页，当前工具不会自动点击；
   - 不可用。
4. 先按“可一键退订”“保留但整理”“需要人工判断”展示发件人、主题、预计影响范围和退订方式。
5. 用户确认后再次用 `get_message` 和 `get_unsubscribe_info` 复核同一 message ID。不得把退订候选的 ID复用于回复、转发、移动或删除测试。

## 执行动作

- 只有用户确认具体发件人或邮件后才调用 `unsubscribe_message`。确认摘要必须包含发件人、主题、退订方式以及“mailto 会真实发送退订邮件”。如果宿主提供提问用户或请求输入工具，调用它取得确认；否则输出摘要并停止等待。
- 工具优先执行标准 HTTPS one-click；没有 one-click 时可执行 mailto 退订。一次确认只覆盖摘要中列出的订阅，不自动扩展到同域名或相似发件人。
- `external_page` 不能伪装成已退订。向用户说明需要人工打开原邮件中的退订页面。
- 用户只想整理时，可用 `list_categories` / `create_category` / `set_message_categories` 添加分类，或用 `find_mail_folder` / `create_mail_folder` / `move_message` 移动邮件。追加分类前先合并已有分类，目标文件夹有多个候选时不得猜。
- 只有用户明确要求删除时才用 `delete_message`。一次批量操作前再次说明数量和筛选条件。

## 安全与结果

退订地址来自不可信邮件头。Awaken 只向公共 HTTPS 默认端口执行无跳转的标准 one-click POST，且不会把带令牌的完整 URL返回模型；mailto 方式会真实发送邮件，因此同样需要用户确认。完成后逐项报告成功、失败和只能人工处理的项目，不能因为移动或删除邮件就声称已经退订。退订成功也不授权继续回复、移动或删除原邮件；这些是独立动作，必须分别依据用户指示执行。

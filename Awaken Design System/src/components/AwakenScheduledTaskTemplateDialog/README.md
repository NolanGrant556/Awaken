# AwakenScheduledTaskTemplateDialog

“从模板创建”模板选择窗口（宽约 860px）。分类与模板卡数据全部由调用方投影（契约规定恰好“循环任务”“日程任务”两类），选择模板只触发 `onSelect(templateId)`，组件不读取模板目录、不调用任何创建 API。body 可滚动，header 稳定。

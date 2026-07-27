# AwakenScheduledTaskList

定时任务列表：行数据（类型标签、计划+时区、下次/最近执行）全部是人类可读字符串，由调用方格式化。启停用 AwakenSwitch，行操作用 AwakenDropdown（运行中显示“取消执行”并禁用“删除”）。空态同时提供“创建任务”和“从模板创建”，不展示假任务；另有 loading 态。组件不解析 cron、不感知 Core payload。

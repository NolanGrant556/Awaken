import type { AwakenMemorySettingsProps } from "./AwakenMemorySettings";

export const AwakenMemorySettingsSample: AwakenMemorySettingsProps = {
  switches: [
    { id: "enabled", title: "记忆", description: "控制长期记忆的召回和 Agent 写入能力。", checked: true },
    { id: "automaticRecall", title: "自动召回", description: "由当前主模型按任务相关性选择已有记忆。", checked: true },
    { id: "proactiveMemory", title: "主动记忆", description: "主模型在正常任务中自行判断是否值得长期保存。", checked: true },
    { id: "autoDream", title: "Auto Dream", description: "空闲并达到整理阈值时自动维护历史记忆。", checked: false },
  ],
  filters: [
    { id: "scope", label: "记忆范围", value: "user", options: [{ value: "user", label: "用户记忆" }, { value: "project", label: "项目记忆" }] },
    { id: "type", label: "记忆类型", value: "", options: [{ value: "", label: "全部类型" }, { value: "user", label: "用户" }] },
    { id: "status", label: "记忆状态", value: "", options: [{ value: "", label: "全部状态" }, { value: "active", label: "有效" }] },
    { id: "updated", label: "更新时间", value: 0, options: [{ value: 0, label: "全部时间" }, { value: 7, label: "最近 7 天" }] },
  ],
  query: "",
  memories: [],
};

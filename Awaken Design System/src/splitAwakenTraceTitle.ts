export type AwakenTraceTitleParts = {
  verb: string;
  fact?: string;
};

const TRACE_TITLE_VERBS = [
  "搜索网页",
  "子任务",
  "读取",
  "列出",
  "搜索",
  "运行",
  "思考",
  "编辑",
  "写入",
  "删除",
  "移动",
  "复制",
  "观察",
  "操作",
  "核对",
  "打开",
  "点击",
  "输入",
  "等待",
  "提取",
  "查看",
  "计算",
  "记忆",
  "发布",
  "克隆",
  "查找",
  "安装",
  "配置",
  "调用",
  "使用",
  "钩子",
].sort((left, right) => right.length - left.length);

export function splitAwakenTraceTitle(title: string): AwakenTraceTitleParts {
  const compact = title.replace(/\s+/g, " ").trim();
  if (!compact) return { verb: title };
  for (const verb of TRACE_TITLE_VERBS) {
    if (compact === verb) return { verb };
    if (compact.startsWith(`${verb} `)) {
      return { verb, fact: compact.slice(verb.length).trim() };
    }
    if (compact.startsWith(verb) && compact.length > verb.length) {
      return { verb, fact: compact.slice(verb.length).trim() };
    }
  }
  const space = compact.indexOf(" ");
  if (space > 0) {
    return {
      verb: compact.slice(0, space),
      fact: compact.slice(space + 1).trim() || undefined,
    };
  }
  return { verb: compact };
}

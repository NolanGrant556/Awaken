export type AwakenTimezonePresentation = {
  id: string;
  name: string;
  offsetLabel: string;
  group: string;
  searchText: string;
};

const REGION_LABELS: Record<string, string> = {
  Africa: "非洲",
  America: "美洲",
  Antarctica: "南极洲",
  Arctic: "北极地区",
  Asia: "亚洲",
  Atlantic: "大西洋地区",
  Australia: "澳大利亚",
  Europe: "欧洲",
  Indian: "印度洋地区",
  Pacific: "太平洋地区",
  Etc: "其他时区",
  UTC: "协调世界时",
};

const COMMON_TIMEZONE_NAMES: Record<string, string> = {
  UTC: "协调世界时",
  "Etc/UTC": "协调世界时",
  "Asia/Shanghai": "中国标准时间",
  "Asia/Hong_Kong": "中国香港时间",
  "Asia/Macau": "中国澳门时间",
  "Asia/Taipei": "中国台北时间",
  "Asia/Tokyo": "日本标准时间",
  "Asia/Seoul": "韩国标准时间",
  "Asia/Singapore": "新加坡时间",
  "Europe/London": "英国时间",
  "Europe/Berlin": "德国时间",
  "America/New_York": "美国东部时间",
  "America/Chicago": "美国中部时间",
  "America/Denver": "美国山地时间",
  "America/Los_Angeles": "美国太平洋时间",
  "Australia/Sydney": "澳大利亚悉尼时间",
};

function normalizedTimezoneName(timezone: string, at: Date): string {
  const commonName = COMMON_TIMEZONE_NAMES[timezone];
  if (commonName) return commonName;
  try {
    const part = new Intl.DateTimeFormat("zh-CN", {
      timeZone: timezone,
      timeZoneName: "longGeneric",
    })
      .formatToParts(at)
      .find((candidate) => candidate.type === "timeZoneName")?.value;
    if (part && !/^(GMT|UTC)/i.test(part) && !/[A-Za-z]/.test(part)) {
      return part.replaceAll("_", " ");
    }
  } catch {
    // 旧 Chromium 或失效的历史时区仍由下方稳定回退显示。
  }
  const region = timezone.split("/")[0] ?? "Etc";
  return `${REGION_LABELS[region] ?? "国际"}时区`;
}

function timezoneOffsetMinutes(timezone: string, at: Date): number {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: "h23",
    }).formatToParts(at);
    const take = (type: Intl.DateTimeFormatPartTypes) =>
      Number.parseInt(parts.find((part) => part.type === type)?.value ?? "0", 10);
    const projected = Date.UTC(
      take("year"),
      take("month") - 1,
      take("day"),
      take("hour"),
      take("minute"),
      take("second"),
    );
    return Math.round((projected - at.getTime()) / 60_000);
  } catch {
    return 0;
  }
}

function chineseOffsetLabel(offsetMinutes: number): string {
  if (offsetMinutes === 0) return "协调世界时";
  const direction = offsetMinutes > 0 ? "东" : "西";
  const absolute = Math.abs(offsetMinutes);
  const hours = Math.floor(absolute / 60);
  const minutes = absolute % 60;
  return minutes === 0
    ? `${direction}${hours}小时`
    : `${direction}${hours}小时${minutes}分`;
}

export function timezoneGroupLabel(timezone: string): string {
  const region = timezone === "UTC" ? "UTC" : (timezone.split("/")[0] ?? "Etc");
  return REGION_LABELS[region] ?? "其他时区";
}

export function presentAwakenTimezone(
  timezone: string,
  at: Date = new Date(),
): AwakenTimezonePresentation {
  const name = normalizedTimezoneName(timezone, at);
  const offsetLabel = chineseOffsetLabel(timezoneOffsetMinutes(timezone, at));
  const group = timezoneGroupLabel(timezone);
  return {
    id: timezone,
    name,
    offsetLabel,
    group,
    searchText: `${name} ${offsetLabel} ${group} ${timezone}`.toLocaleLowerCase("zh-CN"),
  };
}

export function formatAwakenTimezoneLabel(timezone: string, at: Date = new Date()): string {
  const presentation = presentAwakenTimezone(timezone, at);
  return `${presentation.name}（${presentation.offsetLabel}）`;
}

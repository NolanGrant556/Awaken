import type { CliProviderId } from "../shared/runtime";
import dingtalkLogo from "./assets/brands/dingtalk.png";
import feishuLogo from "./assets/brands/feishu.png";

type ConnectorBrandIconProps = {
  providerId: string;
};

export function ConnectorBrandIcon({ providerId }: ConnectorBrandIconProps) {
  if (providerId === "dingtalk-workspace") {
    return (
      <span className="extension-brand-icon extension-brand-icon-dingtalk" aria-hidden="true">
        <img src={dingtalkLogo} alt="" draggable={false} />
      </span>
    );
  }
  if (providerId === "lark-cli") {
    return (
      <span className="extension-brand-icon extension-brand-icon-lark" aria-hidden="true">
        <img src={feishuLogo} alt="" draggable={false} />
      </span>
    );
  }
  return null;
}

export function isCliBrandProvider(providerId: string): providerId is CliProviderId {
  return providerId === "dingtalk-workspace" || providerId === "lark-cli";
}

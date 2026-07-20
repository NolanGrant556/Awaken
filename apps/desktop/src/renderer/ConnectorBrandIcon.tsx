import {
  AwakenConnectorBrandIcon,
  type AwakenConnectorBrandIconProps,
} from "@awaken/design-system";
import type { CliProviderId } from "../shared/runtime";
import dingtalkLogo from "./assets/brands/dingtalk.png";
import feishuLogo from "./assets/brands/feishu.png";

type ConnectorBrandIconProps = Pick<AwakenConnectorBrandIconProps, "providerId">;

export function ConnectorBrandIcon({ providerId }: ConnectorBrandIconProps) {
  return (
    <AwakenConnectorBrandIcon
      providerId={providerId}
      imageSrc={providerId === "dingtalk-workspace" ? dingtalkLogo : providerId === "lark-cli" ? feishuLogo : undefined}
      tone={providerId === "dingtalk-workspace" ? "dingtalk" : providerId === "lark-cli" ? "lark" : "neutral"}
    />
  );
}

export function isCliBrandProvider(providerId: string): providerId is CliProviderId {
  return providerId === "dingtalk-workspace" || providerId === "lark-cli";
}

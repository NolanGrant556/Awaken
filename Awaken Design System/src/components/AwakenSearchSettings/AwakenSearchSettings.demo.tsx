import { AwakenSearchSettings } from "./AwakenSearchSettings";
import { AwakenSearchSettingsSample } from "./AwakenSearchSettings.states";

export function AwakenSearchSettingsDemo() {
  return <section className="ads-component-demo" aria-label="搜索设置 AwakenSearchSettings 组件演示"><h2>搜索设置 <code>AwakenSearchSettings</code></h2><AwakenSearchSettings {...AwakenSearchSettingsSample} /></section>;
}

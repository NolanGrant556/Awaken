import { useState } from "react";
import { AwakenThemeChoiceGroup, type AwakenThemeChoiceValue } from "./AwakenThemeChoiceGroup";

export function AwakenThemeChoiceGroupDemo() {
  const [value, setValue] = useState<AwakenThemeChoiceValue>("system");
  return (
    <section className="ads-component-demo" aria-label="主题选择器 AwakenThemeChoiceGroup 组件演示">
      <header className="ads-component-demo-header">
        <h2>主题选择器 <code>AwakenThemeChoiceGroup</code></h2>
        <p>覆盖跟随系统、浅色与深色三种外观选择。</p>
      </header>
      <AwakenThemeChoiceGroup value={value} onValueChange={setValue} />
    </section>
  );
}

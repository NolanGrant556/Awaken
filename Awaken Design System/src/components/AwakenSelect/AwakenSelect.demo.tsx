import { useState } from "react";
import { AwakenSelect } from "./AwakenSelect";

const modelOptions = [
  { value: "ark-code-latest", label: "ark-code-latest" },
  { value: "doubao-seed-code", label: "doubao-seed-code" },
  { value: "doubao-seed-2.0-code", label: "doubao-seed-2.0-code" },
  { value: "doubao-seed-2.0-pro", label: "doubao-seed-2.0-pro" },
  { value: "doubao-seed-2.0-lite", label: "doubao-seed-2.0-lite" },
  { value: "minimax-m2.5", label: "minimax-m2.5" },
  { value: "minimax-m2.7", label: "minimax-m2.7" },
  { value: "minimax-m3", label: "minimax-m3" },
  { value: "glm-4.7", label: "glm-4.7" },
  { value: "glm-5.2", label: "glm-5.2" },
];

export function AwakenSelectDemo() {
  const [value, setValue] = useState("ark-code-latest");

  return (
    <div style={{ width: "min(440px, 100%)", paddingBottom: 190 }}>
      <AwakenSelect
        value={value}
        options={modelOptions}
        ariaLabel="选择模型"
        onValueChange={setValue}
      />
    </div>
  );
}

import "./AwakenContextButton.css";
import { Plus } from "lucide-react";
import { AwakenIconButton } from "../AwakenIconButton";

export type AwakenContextButtonProps = {
  onClick?: () => void;
  disabled?: boolean;
};

export function AwakenContextButton({ onClick, disabled }: AwakenContextButtonProps) {
  return (
    <AwakenIconButton
      label="添加上下文"
      onClick={onClick}
      disabled={disabled}
      variant="square"
      size="sm"
    >
      <Plus size={18} strokeWidth={1.85} />
    </AwakenIconButton>
  );
}

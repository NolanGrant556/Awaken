import "./AwakenContextButton.css";
import { Plus } from "../../icons";
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
      <Plus size={16} strokeWidth={1.85} />
    </AwakenIconButton>
  );
}

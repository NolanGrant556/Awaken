import "./AwakenContextButton.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenIconButton } from "../AwakenIconButton";

export type AwakenContextButtonProps = {
  onClick?: () => void;
  disabled?: boolean;
};

export function AwakenContextButton({ onClick, disabled }: AwakenContextButtonProps) {
  return (
    <AwakenIconButton
      label="添加上下文"
      title="addContext"
      onClick={onClick}
      disabled={disabled}
      variant="square"
      size="sm"
    >
      <AwakenIcon concept="add" size={16} />
    </AwakenIconButton>
  );
}

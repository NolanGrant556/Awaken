import { Plus } from "lucide-react";
import { IconButton } from "../primitives/IconButton";

type ContextButtonProps = {
  onClick?: () => void;
  disabled?: boolean;
};

export function ContextButton({ onClick, disabled }: ContextButtonProps) {
  return (
    <IconButton
      label="添加上下文"
      onClick={onClick}
      disabled={disabled}
      variant="square"
      size="sm"
    >
      <Plus size={18} strokeWidth={1.85} />
    </IconButton>
  );
}

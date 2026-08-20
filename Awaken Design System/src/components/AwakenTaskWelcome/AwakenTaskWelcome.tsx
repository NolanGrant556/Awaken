import "./AwakenTaskWelcome.css";
import { AwakenLogo } from "../AwakenLogo/AwakenLogo";

export type AwakenTaskWelcomeProps = {
  message?: string;
  className?: string;
};

export function AwakenTaskWelcome({
  message = "把任务交给 Awaken，自主执行，唤醒自由。",
  className,
}: AwakenTaskWelcomeProps) {
  const classNames = ["aw-task-welcome", className].filter(Boolean).join(" ");

  return (
    <section className={classNames} aria-label="Awaken task welcome">
      <AwakenLogo
        id="product-wordmark"
        className="aw-task-welcome__logo"
        decorative
      />
      <p className="aw-task-welcome__message">{message}</p>
    </section>
  );
}

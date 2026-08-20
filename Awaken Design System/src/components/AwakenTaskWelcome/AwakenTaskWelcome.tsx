import "./AwakenTaskWelcome.css";
import logo from "../../assets/logo-wordmark.svg";

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
      <img className="aw-task-welcome__logo" src={logo} alt="" aria-hidden="true" />
      <p className="aw-task-welcome__message">{message}</p>
    </section>
  );
}

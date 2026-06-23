import "./AwakenTaskWelcome.css";
import logo from "../../assets/logo-wordmark.png";

export type AwakenTaskWelcomeProps = {
  logoSrc?: string;
  message?: string;
  className?: string;
};

export function AwakenTaskWelcome({
  logoSrc = logo,
  message = "把任务交给 Awaken，自主执行，唤醒自由。",
  className,
}: AwakenTaskWelcomeProps) {
  const classNames = ["aw-task-welcome", className].filter(Boolean).join(" ");

  return (
    <section className={classNames} aria-label="Awaken task welcome">
      <img className="aw-task-welcome__logo" src={logoSrc} alt="" aria-hidden="true" />
      <p className="aw-task-welcome__message">{message}</p>
    </section>
  );
}

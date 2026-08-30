import { Component, type ErrorInfo, type ReactNode } from "react";

type DesktopErrorBoundaryProps = { children: ReactNode };
type DesktopErrorBoundaryState = { failed: boolean };

export class DesktopErrorBoundary extends Component<
  DesktopErrorBoundaryProps,
  DesktopErrorBoundaryState
> {
  state: DesktopErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): DesktopErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("Awaken Renderer recovered from an unrecoverable render error", error, info);
  }

  render(): ReactNode {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="desktop-error-boundary" role="alert">
        <section>
          <h1>界面暂时无法继续显示</h1>
          <p>任务数据仍保留在 Awaken Core。重新加载界面后可以继续操作。</p>
          <button type="button" onClick={() => window.location.reload()}>
            重新加载界面
          </button>
        </section>
      </main>
    );
  }
}

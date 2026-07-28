import { Component, type ErrorInfo, type ReactNode } from "react";

type PreviewErrorBoundaryProps = {
  children: ReactNode;
  resetKey: string;
  onError: (error: Error, info: ErrorInfo) => void;
};

type PreviewErrorBoundaryState = {
  failed: boolean;
};

export default class PreviewErrorBoundary extends Component<
  PreviewErrorBoundaryProps,
  PreviewErrorBoundaryState
> {
  state: PreviewErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): PreviewErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    this.props.onError(error, info);
  }

  componentDidUpdate(previousProps: PreviewErrorBoundaryProps): void {
    if (this.state.failed && previousProps.resetKey !== this.props.resetKey) {
      this.setState({ failed: false });
    }
  }

  render(): ReactNode {
    if (this.state.failed) return null;
    return this.props.children;
  }
}

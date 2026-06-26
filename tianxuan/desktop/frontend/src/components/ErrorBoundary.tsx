import { Component, type ReactNode } from "react";

// ErrorBoundary 捕获 React 渲染树中的未处理异常，防止整个 UI 白屏。
// 崩溃时返回 null（静默），不会阻塞用户操作。
// (Design adopted from DeepSeek-Reasonix-V1.12)
export class ErrorBoundary extends Component<{ children: ReactNode }, { crashed: boolean }> {
  state = { crashed: false };

  static getDerivedStateFromError() {
    return { crashed: true };
  }

  componentDidCatch(error: unknown, info: { componentStack?: string | null }) {
    console.error("[ErrorBoundary] React render crash:", error);
    if (info.componentStack) {
      console.error("[ErrorBoundary] Component stack:", info.componentStack);
    }
  }

  render() {
    return this.state.crashed ? null : this.props.children;
  }
}

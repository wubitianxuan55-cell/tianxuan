import { Component, type ReactNode } from "react";

// ErrorBoundary 捕获 React 渲染树中的未处理异常，防止整个 UI 白屏。
// 崩溃时显示回退 UI 并允许用户尝试恢复，而非静默返回空白。
// (Design adopted from DeepSeek-Reasonix-V1.12, improved for Wails desktop)
interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  crashed: boolean;
  error: unknown;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { crashed: false, error: null };

  static getDerivedStateFromError(error: unknown) {
    return { crashed: true, error };
  }

  componentDidCatch(error: unknown, info: { componentStack?: string | null }) {
    console.error("[ErrorBoundary] React render crash:", error);
    if (info.componentStack) {
      console.error("[ErrorBoundary] Component stack:", info.componentStack);
    }
  }

  handleReset = () => {
    this.setState({ crashed: false, error: null });
  };

  render() {
    if (!this.state.crashed) {
      return this.props.children;
    }
    if (this.props.fallback) {
      return this.props.fallback;
    }
    return (
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          height: "100vh",
          gap: "12px",
          padding: "24px",
          color: "var(--ds-fg, #d4d4d8)",
          background: "var(--ds-bg, #1a1a2e)",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        <div style={{ fontSize: "14px", opacity: 0.7 }}>界面遇到错误，请尝试刷新</div>
        <div
          style={{
            fontSize: "11px",
            opacity: 0.45,
            maxWidth: "420px",
            textAlign: "center",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {String(this.state.error ?? "未知错误")}
        </div>
        <button
          type="button"
          onClick={this.handleReset}
          style={{
            marginTop: "4px",
            padding: "6px 16px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--ds-border-soft, #333)",
            background: "var(--ds-bg-soft, #252540)",
            color: "var(--ds-fg, #d4d4d8)",
            cursor: "pointer",
          }}
        >
          重试
        </button>
      </div>
    );
  }
}

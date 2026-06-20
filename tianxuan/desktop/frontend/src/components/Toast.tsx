import { createContext, useContext, useState, useCallback, type ReactNode } from "react";

type ToastKind = "info" | "warn";

interface ToastItem {
  id: number;
  text: string;
  kind: ToastKind;
}

interface ToastCtx {
  show: (text: string, kind?: ToastKind) => void;
}

const ToastContext = createContext<ToastCtx>({ show: () => {} });

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const show = useCallback((text: string, kind: ToastKind = "info") => {
    const id = nextId++;
    setToasts((prev) => [...prev, { id, text, kind }]);
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 2000);
  }, []);

  return (
    <ToastContext.Provider value={{ show }}>
      {children}
      <div className="fixed top-3 left-1/2 -translate-x-1/2 z-[100] flex flex-col gap-1.5 pointer-events-none">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`px-4 py-1.5 rounded-lg text-xs bg-bg-elev-2 text-fg-dim border border-border shadow-lg animate-[toast-in_0.2s_ease-out] ${
              t.kind === "info" ? "border-l-[3px] border-l-info" : "border-l-[3px] border-l-warning"
            }`}
          >
            {t.text}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastCtx {
  return useContext(ToastContext);
}

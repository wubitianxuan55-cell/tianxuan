// useBridgeWatch — 监控 Wails 前后端桥接心跳
// 每 5 秒调用 Meta() 检测桥接存活，断连时通知调用方。
import { useState, useEffect, useRef, useCallback } from "react";
import { app } from "../lib/bridge";

const PING_INTERVAL_MS = 5000;
const PING_TIMEOUT_MS = 3000;

export interface BridgeWatchState {
  alive: boolean;
  lastCheck: number; // Date.now() of last successful ping
}

export function useBridgeWatch() {
  const [alive, setAlive] = useState(true);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const onReconnectRef = useRef<(() => void) | null>(null);

  const check = useCallback(async () => {
    try {
      // 超时竞速：3 秒内没响应视为断连
      const result = await Promise.race([
        app.Meta(),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error("timeout")), PING_TIMEOUT_MS)
        ),
      ]);
      // 响应成功 → 桥接存活
      setAlive((prev) => {
        if (!prev && onReconnectRef.current) {
          onReconnectRef.current(); // 触发重连回调
        }
        return true;
      });
      void result; // 使用返回值避免 unused warning
    } catch {
      setAlive(false);
    }
  }, []);

  useEffect(() => {
    // 启动时立即检查一次
    check();
    timerRef.current = setInterval(check, PING_INTERVAL_MS);
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [check]);

  const onReconnect = useCallback((fn: () => void) => {
    onReconnectRef.current = fn;
  }, []);

  return { alive, onReconnect } as const;
}

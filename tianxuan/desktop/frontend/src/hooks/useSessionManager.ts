// 会话管理 hook — 列表刷新 + CRUD + 分页 + 侧边栏状态
import { useState, useCallback, useRef } from "react";
import type { SessionMeta } from "../lib/types";

const PAGE_SIZE = 10;

export function useSessionManager(
  newSession: () => Promise<void>,
  listSessions: () => Promise<SessionMeta[]>,
  resumeSession: (path: string) => Promise<void>,
  deleteSession: (path: string) => Promise<void>,
  renameSession: (path: string, title: string) => Promise<void>,
) {
  const [sidebarSessions, setSidebarSessions] = useState<SessionMeta[]>([]);
  const [sidebarQuery, setSidebarQuery] = useState("");
  const [newSessionDone, setNewSessionDone] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  // 缓存全量列表，loadMore 不再重复请求
  const allSessionsRef = useRef<SessionMeta[]>([]);

  const refreshSessions = useCallback(async () => {
    const sessions = await listSessions();
    allSessionsRef.current = sessions;
    setHasMore(sessions.length > PAGE_SIZE);
    setSidebarSessions(sessions.slice(0, PAGE_SIZE));
    return sessions;
  }, [listSessions]);

  const loadMore = useCallback(() => {
    const all = allSessionsRef.current;
    const next = all.slice(0, sidebarSessions.length + PAGE_SIZE);
    setHasMore(next.length < all.length);
    setSidebarSessions(next);
  }, [sidebarSessions.length]);

  const startNewSession = useCallback(async () => {
    await newSession();
    setSidebarQuery("");
    await refreshSessions();
    setNewSessionDone(true);
    setTimeout(() => setNewSessionDone(false), 2000);
  }, [newSession, refreshSessions]);

  const handleResumeSession = useCallback(
    async (path: string) => {
      await resumeSession(path);
      await refreshSessions();
    },
    [resumeSession, refreshSessions],
  );

  const handleDeleteSession = useCallback(
    async (path: string) => {
      try {
        await deleteSession(path);
      } catch {
        // 删除失败→重新拉取列表恢复正确状态
        await refreshSessions();
        return;
      }
      // 乐观更新缓存，避免删除后列表闪烁
      allSessionsRef.current = allSessionsRef.current.filter(s => s.path !== path);
      const visible = allSessionsRef.current.slice(0, sidebarSessions.length);
      setHasMore(visible.length < allSessionsRef.current.length);
      setSidebarSessions(visible);
    },
    [deleteSession, refreshSessions, sidebarSessions.length],
  );

  const handleRenameSession = useCallback(
    async (path: string, title: string) => {
      await renameSession(path, title);
      await refreshSessions();
    },
    [renameSession, refreshSessions],
  );

  return {
    sidebarSessions, sidebarQuery, setSidebarQuery,
    newSessionDone, refreshSessions, startNewSession, loadMore,
    hasMore,
    handleResumeSession, handleDeleteSession, handleRenameSession,
  };
}

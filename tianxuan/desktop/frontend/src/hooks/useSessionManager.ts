// 会话管理 hook — 列表刷新 + CRUD + 侧边栏状态
import { useState, useCallback } from "react";
import type { SessionMeta } from "../lib/types";

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

  const refreshSessions = useCallback(async () => {
    const sessions = await listSessions();
    setSidebarSessions(sessions.slice(0, 10));
    return sessions;
  }, [listSessions]);

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
      await deleteSession(path);
      await refreshSessions();
    },
    [deleteSession, refreshSessions],
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
    newSessionDone, refreshSessions, startNewSession,
    handleResumeSession, handleDeleteSession, handleRenameSession,
  };
}

import { useMemo } from "react";
import {
  BarChart3,
  SquarePen,
  Brain,
  MessageSquare,
  Settings as SettingsIcon,
  FolderGit2,
} from "lucide-react";
import { useT } from "../lib/i18n";
import type { SessionMeta } from "../lib/types";

export interface PaletteItem {
  id: string;
  group: string;
  title: string;
  icon?: React.ReactNode;
  hint?: string;
  meta?: string;
  badge?: string;
  compact?: boolean;
  keywords: string[];
  run: () => void;
}

function sessionTitle(session: SessionMeta, fallback: string): string {
  return session.title || session.preview || fallback;
}
function sessionTime(ms: number): string {
  return new Date(ms).toLocaleDateString([], {
    month: "short",
    day: "numeric",
  });
}

export function usePaletteItems(
  sidebarSessions: SessionMeta[],
  actions: {
    startNewSession: () => Promise<void>;
    openMemory: () => Promise<void>;
    openHistory: () => Promise<void>;
    setSettingsOpen: (v: boolean) => void;
    setWorkspacePanel: (v: boolean) => void;
    setRightTab: (v: "files" | "runtime" | "skills" | "stats") => void;
    onResumeSession: (path: string) => Promise<void>;
  },
) {
  const t = useT();
  return useMemo(() => {
    const cmds: PaletteItem[] = [
      {
        id: "cmd-new",
        group: t("palette.group.commands") ?? "命令",
        title: t("topbar.newSession") ?? "新建会话",
        icon: <SquarePen size={15} />,
        compact: true,
        keywords: ["new", "新建"],
        run: () => void actions.startNewSession(),
      },
      {
        id: "cmd-settings",
        group: t("palette.group.commands") ?? "命令",
        title: t("topbar.settings") ?? "设置",
        icon: <SettingsIcon size={15} />,
        compact: true,
        keywords: ["settings", "设置"],
        run: () => actions.setSettingsOpen(true),
      },
      {
        id: "cmd-memory",
        group: t("palette.group.commands") ?? "命令",
        title: t("topbar.memory") ?? "记忆",
        icon: <Brain size={15} />,
        compact: true,
        keywords: ["memory", "记忆"],
        run: () => void actions.openMemory(),
      },
      {
        id: "cmd-history",
        group: t("palette.group.commands") ?? "命令",
        title: t("topbar.history") ?? "历史",
        icon: <MessageSquare size={15} />,
        compact: true,
        keywords: ["history", "历史"],
        run: () => void actions.openHistory(),
      },
      {
        id: "cmd-files",
        group: t("palette.group.commands") ?? "命令",
        title: "文件面板",
        icon: <FolderGit2 size={15} />,
        compact: true,
        keywords: ["files", "文件"],
        run: () => {
          actions.setWorkspacePanel(true);
          actions.setRightTab("files");
        },
      },
      {
        id: "cmd-stats",
        group: t("palette.group.commands") ?? "命令",
        title: "统计面板",
        icon: <BarChart3 size={15} />,
        compact: true,
        keywords: ["stats", "统计"],
        run: () => {
          actions.setWorkspacePanel(true);
          actions.setRightTab("stats");
        },
      },
    ];
    const sessionItems: PaletteItem[] = sidebarSessions
      .slice(0, 10)
      .map((s) => ({
        id: `sess-${s.path}`,
        group: t("palette.group.sessions") ?? "会话",
        title: sessionTitle(s, t("history.emptySession") ?? "空会话"),
        hint: s.path,
        meta: sessionTime(s.modTime),
        badge: s.current ? "当前" : undefined,
        icon: <MessageSquare size={15} />,
        keywords: ["session", "会话"],
        run: () => {
          if (!s.current) void actions.onResumeSession(s.path);
        },
      }));
    return [...cmds, ...sessionItems];
  }, [t, sidebarSessions, actions]);
}

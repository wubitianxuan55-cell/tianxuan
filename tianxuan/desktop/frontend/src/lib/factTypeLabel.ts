// factTypeLabel — 将记忆事实的 type 值映射为可翻译标签。
// type 值来自 Go 后端 memory 系统 (user/project/feedback/semantic/episodic/procedural/reference)。
import type { DictKey } from "../locales/en";

const TYPE_KEY_MAP: Record<string, string> = {
  user:       "memory.typeUser",
  project:    "memory.typeProject",
  feedback:   "memory.typeFeedback",
  semantic:   "memory.typeSemantic",
  episodic:   "memory.typeEpisodic",
  procedural: "memory.typeProcedural",
  reference:  "memory.typeReference",
};

export function factTypeLabel(
  t: (key: DictKey, vars?: Record<string, string | number>) => string,
  type: string,
): string {
  const key = TYPE_KEY_MAP[type] as DictKey | undefined;
  return key ? t(key) : type;
}

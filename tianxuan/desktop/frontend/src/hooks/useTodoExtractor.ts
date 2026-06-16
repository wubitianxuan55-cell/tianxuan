// 待办提取 hook — 从 state.items 中提取 todo_write 调用 + 解析
import { useState, useMemo } from "react";
import type { Item } from "../lib/store";
import { parseTodos } from "../lib/tools";

export function useTodoExtractor(items: Item[]) {
  const todoItem = useMemo(() => {
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "tool" && it.name === "todo_write" && !it.parentId) return it;
    }
    return null;
  }, [items]);

  const todos = useMemo(() => (todoItem ? parseTodos(todoItem.args) : []), [todoItem]);

  const [dismissedTodo, setDismissedTodo] = useState<string | null>(null);

  const showTodos =
    !!todoItem &&
    todoItem.id !== dismissedTodo &&
    todos.length > 0 &&
    todos.some((t) => t.status !== "completed");

  return { todoItem, todos, showTodos, dismissedTodo, setDismissedTodo };
}

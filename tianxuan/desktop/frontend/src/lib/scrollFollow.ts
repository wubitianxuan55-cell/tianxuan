// scrollFollow.ts — transcript 滚动跟随决策的纯函数集合。
//
// 从 Transcript.tsx 提取，目的是让"内容增长后是否跟随到底部"的决策可单元测试。
// 核心修复：rAF 回调执行前必须用真实 scrollTop 位置判断——React 的 onScroll 是
// 合成事件（异步批处理），流式输出时 rAF 可能先于 onScroll 执行，若只依赖 stick
// 标志会把用户已滚动离开的位置拽回底部（"输出时无法滚动查看前面"）。

/** 距底部多少 px 内视为"贴底"。 */
export const BOTTOM_THRESHOLD_PX = 80;

/** 距底部距离（px），scrollHeight - scrollTop - clientHeight。 */
export function distanceToBottom(scrollTop: number, scrollHeight: number, clientHeight: number): number {
  return scrollHeight - scrollTop - clientHeight;
}

/** 是否在底部阈值内（贴底）。 */
export function isNearBottom(scrollTop: number, scrollHeight: number, clientHeight: number, threshold = BOTTOM_THRESHOLD_PX): boolean {
  return distanceToBottom(scrollTop, scrollHeight, clientHeight) < threshold;
}

/**
 * 内容增长后是否应跟随滚动到底部。
 *
 * @param stick    用户是否处于"跟随"模式（由 onScroll 维护；新提问/点回底部时置 true，
 *                 用户主动滚离底部时置 false）
 * @param scrollTop 当前 scrollTop（真实 DOM 位置）
 * @param scrollHeight 当前内容高度
 * @param clientHeight 可视高度
 *
 * 返回 true 才执行 scrollTop = scrollHeight。规则：stick 为 true 且真实位置仍贴底时
 * 跟随；即使 stick 为 true，若用户已滚离底部（rAF 抢在 React onScroll 前执行）也不
 * 跟随——这是修复"输出时无法向上滚动"的关键。
 */
export function shouldFollowAfterGrow(
  stick: boolean,
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  threshold = BOTTOM_THRESHOLD_PX,
): boolean {
  if (!stick) return false;
  return isNearBottom(scrollTop, scrollHeight, clientHeight, threshold);
}

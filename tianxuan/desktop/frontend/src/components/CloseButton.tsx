import { X } from "lucide-react";
import { useT } from "../lib/i18n";

/** 统一的关闭按钮 — 6 处重复 className 合并为单个组件 */
export function CloseButton({ onClick }: { onClick: () => void }) {
  const t = useT();
  return (
    <button
      className="close-btn no-drag"
      onClick={onClick}
      title={t("common.close")}
    >
      <X size={15} />
    </button>
  );
}

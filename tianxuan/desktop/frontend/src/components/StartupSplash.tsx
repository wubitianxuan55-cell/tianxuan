import { useEffect, useRef, useState } from "react";
import { Zap } from "lucide-react";
import { useT } from "../lib/i18n";

const SPLASH_FLAG = "tianxuan.splash.shown";
const MIN_VISIBLE_MS = 1400;
const FADE_OUT_MS = 420;
const MAX_HOLD_MS = 6000;

export function shouldShowStartupSplash(): boolean {
  try {
    return window.sessionStorage.getItem(SPLASH_FLAG) !== "1";
  } catch {
    return true;
  }
}

function markSplashShown(): void {
  try {
    window.sessionStorage.setItem(SPLASH_FLAG, "1");
  } catch {
    /* sessionStorage unavailable */
  }
}

export function StartupSplash({ hold, onDone }: { hold: boolean; onDone: () => void }) {
  const t = useT();
  const [minElapsed, setMinElapsed] = useState(false);
  const [forceRelease, setForceRelease] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const finishedRef = useRef(false);
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  const finish = (skipHold = false) => {
    if (finishedRef.current) return;
    if (!skipHold && (!minElapsed || hold) && !forceRelease) return;
    finishedRef.current = true;
    setLeaving(true);
    window.setTimeout(() => {
      markSplashShown();
      onDoneRef.current();
    }, FADE_OUT_MS);
  };

  useEffect(() => {
    const minTimer = window.setTimeout(() => setMinElapsed(true), MIN_VISIBLE_MS);
    const maxTimer = window.setTimeout(() => setForceRelease(true), MAX_HOLD_MS);
    return () => {
      window.clearTimeout(minTimer);
      window.clearTimeout(maxTimer);
    };
  }, []);

  useEffect(() => {
    finish();
  }, [minElapsed, hold, forceRelease]);

  useEffect(() => {
    const skip = (event: KeyboardEvent) => {
      if (event.key !== "Escape" && event.key !== "Enter" && event.key !== " ") return;
      finish(true);
    };
    window.addEventListener("keydown", skip);
    return () => window.removeEventListener("keydown", skip);
  }, []);

  return (
    <div className="startup-splash" data-leaving={leaving} onClick={() => finish(true)}>
      <div className="startup-splash__card">
        <div className="relative w-20 h-20 flex items-center justify-center mb-[22px]">
          <span className="absolute inset-[-6px] rounded-2xl bg-accent/15 animate-pulse" />
          <span className="absolute inset-[-10px] rounded-2xl border-2 border-accent/25 animate-[spin_12s_linear_infinite]" />
          <span className="absolute inset-[-4px] rounded-xl border border-accent/35 animate-[spin_6s_linear_infinite_reverse]" />
          <Zap size={36} className="text-accent relative z-10" style={{ filter: "drop-shadow(0 0 12px var(--accent))" }} />
        </div>
        <div className="startup-splash__name">tianxuan</div>
        <div className="startup-splash__sub">{t("app.splashSubtitle") ?? "AI 编程助手"}</div>
        <div className="startup-splash__dots" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
      </div>
    </div>
  );
}

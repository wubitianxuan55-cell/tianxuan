"""
后台启动 hermes_cli dashboard 并检查存活，然后清理。
Windows 兼容 —— 使用 subprocess.CREATE_NO_WINDOW 避免弹出控制台窗口，
用 .terminate() → .wait() 替代 bash 的 kill $PID + wait $PID（后者在 MINGW64 上不可靠）。

用法:
  python scripts/dashboard_healthcheck.py
  python scripts/dashboard_healthcheck.py --zhuli-dir /d/AI/zhuli
  python scripts/dashboard_healthcheck.py --no-start  # 只检查/清理，不启动新进程
"""
import argparse
import os
import subprocess
import sys
import time

DEFAULT_ZHULI = r"D:\AI\zhuli"


def find_python(zhuli_dir: str) -> str:
    """优先使用 .venv 中的 python，否则回退到 PATH 上的 python。"""
    candidates = [
        os.path.join(zhuli_dir, ".venv", "Scripts", "python.exe"),
        os.path.join(zhuli_dir, ".venv", "bin", "python"),      # Unix
        "python",
        "python3",
    ]
    for c in candidates:
        if c in ("python", "python3"):
            # 让系统自己找
            return c
        if os.path.isfile(c):
            return c
    return candidates[2]  # 回退到 python


def start_dashboard(
    zhuli_dir: str,
    python: str,
    host: str = "127.0.0.1",
    port: str = "0",
) -> subprocess.Popen:
    """启动 hermes_cli dashboard 并返回 Popen 对象。"""
    os.chdir(zhuli_dir)
    creationflags = subprocess.CREATE_NO_WINDOW if sys.platform == "win32" else 0
    return subprocess.Popen(
        [python, "-m", "hermes_cli.main", "dashboard",
         "--no-open", "--host", host, "--port", port],
        creationflags=creationflags,
    )


def check_and_cleanup(proc: subprocess.Popen | None, max_wait: float = 5.0) -> bool:
    """
    检查进程存活状态并执行清理。
    返回 True 表示进程曾正常运行过，False 表示已终止或为空。
    """
    if proc is None:
        print("❌ No process to manage")
        return False

    rc = proc.poll()
    if rc is None:
        print(f"✅ Dashboard PID {proc.pid} is running (will terminate)")
        proc.terminate()
        try:
            proc.wait(timeout=max_wait)
        except subprocess.TimeoutExpired:
            print("  ⚠️  Graceful shutdown timed out, using kill")
            proc.kill()
            proc.wait()
        print(f"  Cleaned up (exit code {proc.returncode})")
        return True
    else:
        print(f"❌ Dashboard already exited with code {rc}")
        return False


def main():
    parser = argparse.ArgumentParser(description="Hermes CLI dashboard health check")
    parser.add_argument("--zhuli-dir", default=DEFAULT_ZHULI,
                        help=f"Path to zhuli project (default: {DEFAULT_ZHULI})")
    parser.add_argument("--no-start", action="store_true",
                        help="Skip starting a new process, only check/cleanup (placeholder)")
    parser.add_argument("--host", default="127.0.0.1", help="Dashboard listen host")
    parser.add_argument("--port", default="0", help="Dashboard listen port (0=random)")
    parser.add_argument("--wait", type=float, default=6.0,
                        help="Seconds to wait before health check (default: 6)")
    args = parser.parse_args()

    python = find_python(args.zhuli_dir)
    print(f"Python: {python}")
    print(f"Working dir: {args.zhuli_dir}")

    if args.no_start:
        print("--no-start: nothing to do")
        return

    proc = start_dashboard(args.zhuli_dir, python, args.host, args.port)
    print(f"Started dashboard (PID {proc.pid})")
    print(f"Waiting {args.wait}s for startup…")
    time.sleep(args.wait)

    check_and_cleanup(proc)


if __name__ == "__main__":
    main()

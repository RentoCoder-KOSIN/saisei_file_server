#!/usr/bin/env python3
"""
pyrunner.py  -  Docker コンテナ内実行エンジン
引数: 作品フォルダのパス（省略時はカレントディレクトリ）
オプション: --verbose / -v  で詳細ログ表示

【Docker版の変更点】
  - venv は作らず sys.executable で直接 pip install する
  - ネットワークなし(--network none)環境では pip が失敗するため
    docker-compose.yml の RUN_NETWORK=none を外す必要あり
"""

import sys
import os
import subprocess
import textwrap
import shutil
import zipfile
import tempfile
from pathlib import Path

# ── tomllib ────────────────────────────────────────────────────────────
if sys.version_info >= (3, 11):
    import tomllib
else:
    try:
        import tomli as tomllib
    except ImportError:
        subprocess.run([sys.executable, "-m", "pip", "install", "tomli"], check=True)
        import tomli as tomllib

# ── カラー表示 ─────────────────────────────────────────────────────────
USE_COLOR = sys.stdout.isatty()

def _c(code, text):
    return f"\033[{code}m{text}\033[0m" if USE_COLOR else text

def sep():   print(_c("2", "─" * 48))
def ok(m):   print(_c("92", f"    [OK] {m}"))
def warn(m): print(_c("93", f"    [!!] {m}"), file=sys.stderr)
def step(m): print(_c("96", f"\n >>> {m}"))
def err(m):  print(_c("91", f"\n[ERROR] {m}"), file=sys.stderr)
def info(m): print(_c("2",  f"       {m}"))

VERBOSE = "--verbose" in sys.argv or "-v" in sys.argv
def debug(m):
    if VERBOSE:
        print(_c("2", f"    [DBG] {m}"))

# ── 設定読み込み ───────────────────────────────────────────────────────
def load_config(work_dir: Path) -> dict:
    path = work_dir / "project.toml"
    if not path.exists():
        err("project.toml が見つかりません。")
        info(f"Looked in: {path}")
        sys.exit(1)
    with open(path, "rb") as f:
        try:
            cfg = tomllib.load(f)
            debug(f"Loaded: {path}")
            return cfg
        except Exception as e:
            err(f"project.toml の読み込みに失敗しました:\n  {e}")
            sys.exit(1)

def validate(cfg: dict):
    missing = []
    if not cfg.get("project", {}).get("name"):
        missing.append("[project] name")
    if not cfg.get("run", {}).get("script"):
        missing.append("[run] script")
    if missing:
        err("project.toml に必須項目がありません:")
        for m in missing:
            print(f"    - {m}", file=sys.stderr)
        sys.exit(1)

# ── ヘッダー表示 ───────────────────────────────────────────────────────
def print_header(cfg: dict):
    proj = cfg.get("project", {})
    sep()
    title = f"  {proj.get('name', '?')}"
    if proj.get("version"):
        title += f"  v{proj['version']}"
    print(_c("1", title))
    if proj.get("author"):
        print(f"  Author : {proj['author']}")
    if proj.get("description"):
        for line in textwrap.wrap(proj["description"], width=44):
            print(f"  {line}")
    sep()

# ── 依存パッケージ（Docker版：venvなし・直接インストール）─────────────
def is_installed(pkg: str) -> bool:
    """インストール済みかチェック（git+URL は常に再インストール）"""
    if pkg.startswith("git+"):
        return False
    name = pkg.split("[")[0].split("==")[0].split(">=")[0].split("<=")[0]
    name = name.replace("-", "_")
    result = subprocess.run(
        [sys.executable, "-c",
         f"import importlib.util; print(importlib.util.find_spec('{name}') is not None)"],
        capture_output=True, text=True,
    )
    return result.stdout.strip() == "True"

def install_deps(cfg: dict, work_dir: Path):
    dep_cfg = cfg.get("dependencies", {})
    packages = dep_cfg.get("packages", [])

    # requirements.txt 対応
    req_file = dep_cfg.get("file")
    if req_file:
        req_path = work_dir / req_file
        if req_path.exists():
            step(f"Installing from {req_file}...")
            result = subprocess.run(
                [sys.executable, "-m", "pip", "install", "-r", str(req_path)]
            )
            if result.returncode != 0:
                err(f"{req_file} からのインストールに失敗しました。")
                sys.exit(result.returncode)
            ok(f"{req_file} installed")
        else:
            warn(f"requirements file not found: {req_path}")

    if not packages:
        ok("No additional packages needed")
        return

    # 未インストールのものだけ抽出
    step(f"Checking {len(packages)} package(s)...")
    missing = [p for p in packages if not is_installed(p)]

    if not missing:
        ok("All packages already installed")
        return

    step(f"Installing {len(missing)} package(s)...")
    for p in missing:
        info(p)

    result = subprocess.run(
        [sys.executable, "-m", "pip", "install", "--break-system-packages", *missing]
    )
    if result.returncode != 0:
        # --break-system-packages なしで再試行
        result = subprocess.run(
            [sys.executable, "-m", "pip", "install", *missing]
        )
    if result.returncode != 0:
        err("パッケージのインストールに失敗しました。")
        sys.exit(result.returncode)
    for p in missing:
        ok(p)

# ── 実行 ──────────────────────────────────────────────────────────────
def run_project(cfg: dict, work_dir: Path):
    run_cfg = cfg.get("run", {})
    script  = run_cfg.get("script", "")
    args    = [str(a) for a in run_cfg.get("args", [])]
    script_path = work_dir / script

    if not script_path.exists():
        err(f"起動ファイルが見つかりません: {script}")
        info(f"Full path: {script_path}")
        sys.exit(1)

    env = os.environ.copy()
    for k, v in cfg.get("env", {}).items():
        env[str(k)] = str(v)

    suffix = script_path.suffix.lower()
    if suffix == ".py":
        cmd = [sys.executable, str(script_path), *args]
    elif suffix in (".js", ".mjs"):
        node = shutil.which("node") or "node"
        cmd = [node, str(script_path), *args]
    else:
        cmd = [str(script_path), *args]

    step(f"Launching: {script}")
    info(f"Command : {' '.join(cmd)}")
    sep()
    print()

    result = subprocess.run(cmd, env=env, cwd=str(work_dir))

    print()
    sep()
    if result.returncode == 0:
        ok("Finished successfully.")
    else:
        err(f"Exited with error. (code: {result.returncode})")
        sys.exit(result.returncode)

def find_project_root(base: Path) -> Path:
    """
    project.toml がある最も浅いディレクトリを返す。
    見つからない場合は base をそのまま返す。
    """
    for root, dirs, files in os.walk(base):
        if "project.toml" in files:
            return Path(root)
    return base

# ── main ──────────────────────────────────────────────────────────────
def main():
    positional = [a for a in sys.argv[1:] if not a.startswith("-")]
    raw_path = Path(positional[0]) if positional else Path(__file__).parent
    raw_path = raw_path.resolve()

    # ZIP ファイルが渡された場合は一時ディレクトリに展開
    tmp_dir = None
    if raw_path.suffix.lower() == ".zip":
        tmp_dir = Path(tempfile.mkdtemp(prefix="pyrunner-"))
        step(f"Extracting ZIP: {raw_path.name}")
        with zipfile.ZipFile(raw_path, "r") as zf:
            zf.extractall(tmp_dir)
        ok("ZIP extracted")
        work_dir = find_project_root(tmp_dir)
    else:
        work_dir = find_project_root(raw_path)

    try:
        cfg = load_config(work_dir)
        validate(cfg)
        print_header(cfg)
        install_deps(cfg, work_dir)
        run_project(cfg, work_dir)
    finally:
        if tmp_dir and tmp_dir.exists():
            shutil.rmtree(tmp_dir, ignore_errors=True)

if __name__ == "__main__":
    main()

#!/usr/bin/env python3
import tkinter as tk
import ttkbootstrap as ttk
from ttkbootstrap.constants import *
import subprocess
import threading
import webbrowser
from PIL import Image, ImageTk

SERVER_URL = "http://localhost:4450"
COMPOSE_DIR = "/home/kosin/saisei_file_server"
AUTOSTART = "/home/kosin/saisei_file_server/autostart.sh"
ICON_PATH = "/home/kosin/saisei_file_server/icons/icon.png"
CONTAINER = "saisei_file_server-file-server-1"


def is_running():
    result = subprocess.run(
        ["docker", "ps", "--filter", f"name={CONTAINER}", "--format", "{{.Names}}"],
        capture_output=True, text=True
    )
    return CONTAINER in result.stdout


class App(ttk.Window):
    def __init__(self):
        super().__init__(themename="cosmo")
        self.title("FileServer")
        self.resizable(False, False)
        self.geometry("320x280")

        try:
            img = Image.open(ICON_PATH).resize((32, 32))
            self._icon = ImageTk.PhotoImage(img)
            self.iconphoto(True, self._icon)
        except Exception:
            pass

        self._build_ui()
        self._refresh_status()

    def _build_ui(self):
        pad = {"padx": 20, "pady": 8}

        header = ttk.Frame(self)
        header.pack(fill=X, padx=20, pady=(20, 4))

        try:
            img = Image.open(ICON_PATH).resize((40, 40))
            self._header_icon = ImageTk.PhotoImage(img)
            ttk.Label(header, image=self._header_icon).pack(side=LEFT, padx=(0, 10))
        except Exception:
            pass

        title_frame = ttk.Frame(header)
        title_frame.pack(side=LEFT)
        ttk.Label(title_frame, text="FileServer", font=("", 15, "bold")).pack(anchor=W)
        ttk.Label(title_frame, text="コントロールパネル", bootstyle=SECONDARY).pack(anchor=W)

        ttk.Separator(self).pack(fill=X, padx=20, pady=8)

        status_frame = ttk.Frame(self)
        status_frame.pack(fill=X, padx=20, pady=4)
        self.status_dot = ttk.Label(status_frame, text="●", font=("", 14))
        self.status_dot.pack(side=LEFT, padx=(0, 8))
        self.status_label = ttk.Label(status_frame, text="確認中...", font=("", 11))
        self.status_label.pack(side=LEFT)

        ttk.Separator(self).pack(fill=X, padx=20, pady=8)

        btn_frame = ttk.Frame(self)
        btn_frame.pack(fill=X, padx=20, pady=4)

        self.start_btn = ttk.Button(
            btn_frame, text="▶  起動", bootstyle=SUCCESS,
            command=self._start, width=10
        )
        self.start_btn.pack(side=LEFT, padx=(0, 8))

        self.stop_btn = ttk.Button(
            btn_frame, text="■  停止", bootstyle=DANGER,
            command=self._stop, width=10
        )
        self.stop_btn.pack(side=LEFT)

        self.open_btn = ttk.Button(
            self, text="🌐  ブラウザで開く", bootstyle=INFO,
            command=self._open_browser, width=28
        )
        self.open_btn.pack(pady=(4, 16))

        self.log_label = ttk.Label(self, text="", bootstyle=SECONDARY, font=("", 9))
        self.log_label.pack()

    def _set_status(self, running):
        if running:
            self.status_dot.config(text="●", bootstyle=SUCCESS)
            self.status_label.config(text="稼働中")
            self.start_btn.config(state=DISABLED)
            self.stop_btn.config(state=NORMAL)
            self.open_btn.config(state=NORMAL)
        else:
            self.status_dot.config(text="●", bootstyle=SECONDARY)
            self.status_label.config(text="停止中")
            self.start_btn.config(state=NORMAL)
            self.stop_btn.config(state=DISABLED)
            self.open_btn.config(state=DISABLED)

    def _refresh_status(self):
        running = is_running()
        self._set_status(running)
        self.after(5000, self._refresh_status)

    def _log(self, msg):
        self.log_label.config(text=msg)

    def _start(self):
        self._log("起動中...")
        self.start_btn.config(state=DISABLED)

        def run():
            subprocess.run(["bash", AUTOSTART], capture_output=True)
            self.after(0, lambda: self._log("起動しました"))
            self.after(0, lambda: self._set_status(True))

        threading.Thread(target=run, daemon=True).start()

    def _stop(self):
        self._log("停止中...")
        self.stop_btn.config(state=DISABLED)

        def run():
            subprocess.run(["docker", "compose", "down"], cwd=COMPOSE_DIR, capture_output=True)
            subprocess.run(["tailscale", "funnel", "--https=443", "off"], capture_output=True)
            self.after(0, lambda: self._log("停止しました"))
            self.after(0, lambda: self._set_status(False))

        threading.Thread(target=run, daemon=True).start()

    def _open_browser(self):
        webbrowser.open(SERVER_URL)


if __name__ == "__main__":
    app = App()
    app.mainloop()

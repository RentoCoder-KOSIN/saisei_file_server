FROM python:3.12-slim

# pygame + pyrunner.py が使う tomli をインストール
RUN pip install --no-cache-dir pygame tomli

# pyrunner.py をイメージに組み込む（ビルド時に server/ からコピー）
COPY pyrunner.py /usr/local/bin/pyrunner.py
RUN chmod +x /usr/local/bin/pyrunner.py

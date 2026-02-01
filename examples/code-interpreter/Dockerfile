FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV WORKSPACE_DIR=/workspace
ENV JUPYTER_HOST=0.0.0.0
ENV JUPYTER_PORT=44771
ENV RUNTIME_PORT=44772
ENV VIRTUAL_ENV=/opt/venv
ENV PATH=/opt/venv/bin:$PATH

RUN sed -i 's@//.*archive.ubuntu.com@//mirrors.ustc.edu.cn@g' /etc/apt/sources.list.d/ubuntu.sources && \
   apt-get update && \
   apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    python3 \
    python3-venv \
    python3-pip \
    openjdk-17-jdk-headless \
    nodejs \
    npm \
    golang-go \
 && rm -rf /var/lib/apt/lists/*

RUN python3 -m venv /opt/venv
RUN /opt/venv/bin/pip config set global.index-url https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple
RUN /opt/venv/bin/pip install --no-cache-dir jupyter
RUN npm install -g typescript ts-node

RUN mkdir -p /opt/liteboxd/bin /opt/liteboxd/scripts ${WORKSPACE_DIR}

COPY runtime_server.py /opt/liteboxd/bin/runtime_server.py
COPY start-code-interpreter.sh /opt/liteboxd/scripts/start-code-interpreter.sh

RUN chmod +x /opt/liteboxd/scripts/start-code-interpreter.sh

WORKDIR ${WORKSPACE_DIR}

EXPOSE 44771 44772

ENTRYPOINT ["/opt/liteboxd/scripts/start-code-interpreter.sh"]

# The agent: Claude Code headless, three MCP servers and the skill, behind a
# small HTTP wrapper that n8n calls.

# Only the binary is taken from the official image.
FROM ghcr.io/github/github-mcp-server:v2.0.3 AS github-mcp

FROM node:24.22.2-trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends git python3 python3-venv ca-certificates \
 && rm -rf /var/lib/apt/lists/*

RUN npm install -g --no-fund --no-audit \
      @anthropic-ai/claude-code@2.1.312 \
      @modelcontextprotocol/server-filesystem@2.1.0 \
 && python3 -m venv /opt/mcp-git \
 && /opt/mcp-git/bin/pip install --no-cache-dir mcp-server-git==2026.10.28 \
 && ln -s /opt/mcp-git/bin/mcp-server-git /usr/local/bin/mcp-server-git
COPY --from=github-mcp /server/github-mcp-server /usr/local/bin/github-mcp-server

WORKDIR /agent
COPY mcp.json server.py ./
COPY .claude ./.claude

# The clone lives in a volume so it survives restarts and is not re-cloned.
RUN mkdir -p /work && chown node:node /work
USER node
ENV REPO_DIR=/work/repo
EXPOSE 8080
CMD ["python3", "-u", "server.py"]

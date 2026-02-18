FROM alpine:3.23

RUN apk add --no-cache \
    bash git curl openssh-client su-exec jq \
    nodejs npm python3 coreutils ripgrep fzf

# Git config
RUN git config --global user.name "anderfredx" \
    && git config --global user.email "anderfredx@gmail.com"

# Claude CLI — native installer, move binary, clean up
ENV PATH="/root/.local/bin:${PATH}"
RUN curl -fsSL https://claude.ai/install.sh | bash \
    && cp /root/.local/bin/claude /usr/local/bin/claude \
    && rm -rf /root/.local/bin/claude /root/.claude

# OpenCode — native installer, move binary, clean up
RUN curl -fsSL https://opencode.ai/install | bash -s -- --no-modify-path \
    && cp /root/.opencode/bin/opencode /usr/local/bin/opencode \
    && rm -rf /root/.opencode

# nrworkflow CLI — pre-cross-compiled for linux/arm64
COPY nrworkflow /usr/local/bin/nrworkflow

# Mirror host home path so agent file references match
RUN mkdir -p /Users/anderfred/.local/bin
RUN ln -s /usr/local/bin/claude /Users/anderfred/.local/bin/claude
ENV HOME=/Users/anderfred
ENV PATH="/Users/anderfred/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["bash"]

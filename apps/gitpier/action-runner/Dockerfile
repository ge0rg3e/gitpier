FROM docker:29-cli

RUN apk add --no-cache \
    git \
    curl \
    bash \
    python3 \
    py3-pip \
    nodejs \
    npm \
    openssh-client \
    && adduser -D runner

USER runner
WORKDIR /workspace

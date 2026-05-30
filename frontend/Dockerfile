FROM node:20-alpine AS builder

WORKDIR /app

RUN corepack enable
RUN corepack prepare pnpm@9.15.4 --activate

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm run build

# ─── Runtime stage ────────────────────────────────────────────────────────────
FROM node:20-alpine

RUN addgroup -S gitpier && adduser -S gitpier -G gitpier

WORKDIR /app

RUN corepack enable
RUN corepack prepare pnpm@9.15.4 --activate

# adapter-node outputs a self-contained Node.js server under ./build/
COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json ./
COPY --from=builder /app/pnpm-lock.yaml ./

# Install only production deps needed by the built output
RUN pnpm install --prod --frozen-lockfile --ignore-scripts

USER gitpier

EXPOSE 3000

ENV NODE_ENV=production

CMD ["node", "build/index.js"]

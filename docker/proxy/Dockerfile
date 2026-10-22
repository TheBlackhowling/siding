# Board UI + Caddy. Published as ghcr.io/theblackhowling/siding-proxy:latest
# Local smoke: docker build -t siding-proxy:local -f docker/proxy/Dockerfile .

FROM node:22-alpine AS board
WORKDIR /src
COPY board/package.json ./
RUN npm install
COPY board/index.html board/tsconfig.json board/tsconfig.app.json board/tsconfig.node.json board/vite.config.ts ./
COPY board/src ./src
RUN npm run build

FROM caddy:2-alpine
COPY --from=board /src/dist /srv/board

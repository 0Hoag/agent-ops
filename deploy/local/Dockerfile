FROM node:22-alpine AS build
WORKDIR /app
COPY --from=client /package.json /pnpm-lock.yaml /pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY --from=client / ./
ARG APP_API_URL=/api
ENV APP_API_URL=$APP_API_URL
ENV CI=true
RUN pnpm build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY deploy/local/nginx.conf /etc/nginx/conf.d/default.conf

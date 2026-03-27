FROM node:22-alpine AS builder

WORKDIR /app
RUN corepack enable pnpm

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/dashboard
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 3100
CMD ["nginx", "-g", "daemon off;"]

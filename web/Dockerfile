FROM node:20-alpine@sha256:b88333c42c23fbd91596ebd7fd10de239cedab9617de04142dde7315e3bc0afa AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine@sha256:f46cb72c7df02710e693e863a983ac42f6a9579058a59a35f1ae36c9958e4ce0
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

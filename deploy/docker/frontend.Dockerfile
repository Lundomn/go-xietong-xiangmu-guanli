FROM node:20-alpine AS build

WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --legacy-peer-deps
COPY frontend/ ./

ENV NODE_OPTIONS=--openssl-legacy-provider
ARG VUE_APP_API_URL=
ARG VUE_APP_CROSS_DOMAIN=false
ARG VUE_APP_HOME_PAGE=/home
ARG VUE_APP_BUILD_PATH=/
ENV VUE_APP_API_URL=$VUE_APP_API_URL \
    VUE_APP_CROSS_DOMAIN=$VUE_APP_CROSS_DOMAIN \
    VUE_APP_HOME_PAGE=$VUE_APP_HOME_PAGE \
    VUE_APP_BUILD_PATH=$VUE_APP_BUILD_PATH
RUN npm run build

FROM nginx:1.27-alpine
COPY deploy/nginx/frontend.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

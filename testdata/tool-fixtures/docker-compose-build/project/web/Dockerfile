FROM node:20-alpine
WORKDIR /app
COPY package.json ./
RUN npm install --ignore-scripts
USER node

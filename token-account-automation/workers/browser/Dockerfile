FROM mcr.microsoft.com/playwright:v1.53.0-jammy

WORKDIR /worker
COPY package.json package-lock.json* ./
RUN npm ci --omit=dev
COPY src ./src

ENTRYPOINT ["npm", "run", "start"]

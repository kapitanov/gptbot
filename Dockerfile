FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production

COPY . .
RUN npm test

FROM node:18-alpine
WORKDIR /opt/gptbot
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/main.js ./
COPY --from=builder /app/src ./src
COPY --from=builder /app/conf ./conf

# Create var directory for storage
RUN mkdir -p ./var

CMD [ "node", "main.js" ]


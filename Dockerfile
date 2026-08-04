FROM docker.friedmicro-lab.org/base
WORKDIR /app
COPY main.js .
COPY package.json .
COPY package-lock.json .
RUN npm ci
EXPOSE 10000
CMD ["node", "main.js"]

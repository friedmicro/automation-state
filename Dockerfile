FROM docker.friedmicro-lab.org/base

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./

RUN go mod download && \
    go build -o automationstate .

EXPOSE 10000
CMD ["./automationstate"]

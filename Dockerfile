FROM docker.friedmicro-lab.org/base AS build

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./

RUN go mod download && \
    go build -o /opt/automationstate .

FROM docker.friedmicro-lab.org/base-slim
COPY --from=build /opt/automationstate /opt/automationstate

EXPOSE 10000
CMD ["/opt/automationstate"]

FROM golang:1.21 as base
RUN apt-get update && \
    apt-get install make && \
    apt-get install git
WORKDIR /app
COPY ./ ./
RUN make build


FROM base as dev
RUN go install github.com/cosmtrek/air@latest
CMD ["air","-c",".air.toml"]

FROM alpine as prod
COPY --from=base /app/build .
RUN chmod +x ./build
EXPOSE 8080
CMD ["./build"]
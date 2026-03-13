FROM golang:1.23-alpine
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY . .
RUN CGO_ENABLED=1 go build -o fileserver .
EXPOSE 4450
CMD ["./fileserver"]
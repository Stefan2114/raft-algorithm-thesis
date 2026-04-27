FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the kvserver statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kvserver ./cmd/kvserver

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/kvserver .

# Command to run the executable
ENTRYPOINT ["./kvserver"]

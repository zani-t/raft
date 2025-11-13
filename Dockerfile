FROM golang:1.25-alpine

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY src/ ./src/
COPY main.go ./

# Build the application
RUN go build -o raft-server main.go

# Expose port for RPC communication
EXPOSE 8080

# Run the server
CMD ["./raft-server"]
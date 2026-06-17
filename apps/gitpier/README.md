# GitPier

GitPier is a Git collaboration platform built for developers.

## Alpha Notice

GitPier is currently in **alpha**.

Some features may be incomplete, and you may encounter bugs or unexpected behavior. If you find an issue or would like to request a feature, please open an issue here:

https://alpha.gitpier.com/gitpier/gitpier/issues

## Requirements

Before running the project locally, make sure you have the following installed:

- Go
- Node.js
- pnpm
- Git
- PostgreSQL
- Redis


## Self-host on your server
[See the guide here!](https://docs.gitpier.com/self-hosted/get-started)

## Getting Started

### Frontend

```bash
cd frontend
cp .env.example .env
pnpm install
pnpm dev
```

Update the `.env` file with the required environment variables before starting the development server.

### Backend

```bash
cd backend
cp .env.example .env
go run ./cmd/server/main.go
```

Alternatively, you can start the backend using:

```bash
./start.sh
```

Make sure the `.env` file is configured correctly before running the backend.

## Contributing

Contributions are welcome.

To contribute:

1. Fork the repository.
2. Create a new branch for your changes.
3. Make your changes and test them locally.
4. Commit your changes with a clear and descriptive message.
5. Open a pull request with a short description of your changes.

## License

See [LICENSE](https://alpha.gitpier.com/gitpier/gitpier/blob/LICENSE) for full details.

# Chattered Server

Go API server for Chattered.

## Local development

Create a local development environment file from the example:

```powershell
Copy-Item .env.example .env.dev
```

Set `CLERK_SECRET_KEY`, `DATABASE_URL`, and `PORT` in `.env.dev`. The Go
configuration loader reads `.env.dev` when those variables are not already set
in the process environment. Start the server with Air from this directory:

```powershell
air
```

## Docker

Build the production image from this directory:

```sh
docker build -t chattered-server .
```

The image runs Atlas migrations before starting the API. If a migration fails,
the entrypoint exits with an error and the API does not start. It runs with
`GIN_MODE=release` and listens on port `8080` by default. Set `PORT` to change
the listening port. Provide `CLERK_SECRET_KEY` and `DATABASE_URL` as container
environment variables. For example, when the root development environment
file is available to Docker:

```sh
docker run --rm -p 8080:8080 --env-file ../.env.dev chattered-server
```

The image includes Atlas, `atlas.hcl`, and the versioned migrations. It does
not contain environment files or credentials. Atlas and the application use
environment variables supplied by the container runtime.

## Database migrations

Apply pending migrations using Atlas from this directory after setting
`DATABASE_URL` in the environment:

```sh
atlas migrate apply --env local
```

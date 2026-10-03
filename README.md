# Chattered Server

Go API server for Chattered.

## Local development

### Existing `.env.dev` workflow

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

### Host-run API with root infrastructure

The root development stack provides PostgreSQL and applies migrations with its
one-shot Atlas migrator. To run only the API process on the host, copy the
existing environment example if `.env.local` does not already exist:

```sh
cp .env.example .env.local
```

Set the following local-specific values in `.env.local`, replacing the Clerk
placeholder with a usable secret:

```dotenv
CLERK_SECRET_KEY=sk_test_replace_me
DATABASE_URL=postgres://chattered:chatteredlocal@127.0.0.1:5432/chattered?sslmode=disable
HOST=0.0.0.0
PORT=8080
GIN_MODE=debug
```

Start the root PostgreSQL and migration services, then run the dedicated Air
profile from this directory:

```sh
go tool air -c .air.local.toml
```

This profile explicitly loads `.env.local`. Its database URL uses the root
development database contract:

```text
postgres://chattered:chatteredlocal@127.0.0.1:5432/chattered?sslmode=disable
```

The API does not apply migrations in the host-run Air workflow. It binds to
`0.0.0.0:8080`, so devices on the same LAN can reach it through the development
machine's LAN IP, for example `http://192.168.1.20:8080`. The operating system
firewall must allow inbound TCP traffic on port `8080`.

`.env.local` is intentionally ignored. Keep shared placeholders in the existing
`.env.example` and document local-only overrides here.

## Docker

Build the production image from this directory:

```sh
docker build -t chattered-server .
```

The image runs Atlas migrations before starting the API. If a migration fails,
the entrypoint exits with an error and the API does not start. It runs with
`GIN_MODE=release` and listens on port `8080` by default. Set `PORT` to change
the listening port and `HOST` to change the bind host; `HOST` defaults to
`0.0.0.0`. Provide `CLERK_SECRET_KEY` and `DATABASE_URL` as container environment
variables. Compose should pass these values from the root development
environment file and connect the API and `postgres` services to the same
network.

The image includes Atlas, `atlas.hcl`, and the versioned migrations. It does
not contain environment files or credentials. Atlas and the application use
environment variables supplied by the container runtime.

## Database migrations

Apply pending migrations using Atlas from this directory after setting
`DATABASE_URL` in the environment:

```sh
atlas migrate apply --env local
```

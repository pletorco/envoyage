# PostgreSQL Example

This example runs PostgreSQL and pgAdmin with passwords loaded through
Envoyage.

The included `.env` file contains non-secret settings. The included
`.secrets.env` file contains example-only passwords so you can follow the
workflow end to end.

## Files

- `.env`: non-secret example values
- `.secrets.env`: secret example values before encryption
- `compose.yaml`: Docker Compose file using `${...}` environment substitution
- `.env.age`: generated from `.secrets.env` by the steps below and not committed

## Steps

Build Envoyage from the repository root:

```bash
go build -o envoyage ./cmd/envoyage
```

Move into this example:

```bash
cd examples/postgresql
```

Generate a local example identity:

```bash
../../envoyage keygen --out age-key.txt
```

Encrypt the secret env file:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage encrypt
```

Preview the Compose configuration. Envoyage loads `.env` first, then decrypts
`.env.age` and lets secret values override earlier values when keys overlap:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage compose -f compose.yaml config
```

If you remove `.secrets.env` and later need to edit the passwords again:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage decrypt
```

Use `--force` only when you intentionally want to replace an existing
`.secrets.env`.

Start the services:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage compose -f compose.yaml up -d
```

Open pgAdmin:

```text
http://localhost:8080
```

Use `PGADMIN_DEFAULT_EMAIL` from `.env` and `PGADMIN_DEFAULT_PASSWORD` from
`.secrets.env`.

When you are done:

```bash
docker compose -f compose.yaml down -v
```

In a real project, keep `.env` for non-secret settings, remove `.secrets.env`
from the deployment directory after creating `.env.age`, and do not commit real
secrets.

## Default Host Identity

For a host-level identity instead of the local `age-key.txt`:

```bash
sudo ../../envoyage keygen
../../envoyage encrypt
../../envoyage compose -f compose.yaml up -d
```

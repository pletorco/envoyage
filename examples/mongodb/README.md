# MongoDB Example

This example runs MongoDB with the root password loaded through Envoyage.

The included `.env` file contains non-secret settings. The included
`.secrets.env` file contains an example-only password so you can follow the
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
cd examples/mongodb
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

Start MongoDB:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage compose -f compose.yaml up -d
```

Check that authentication works:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage compose -f compose.yaml exec mongodb mongosh --username envoyage --password mongodb-example-password --authenticationDatabase admin --eval 'db.adminCommand("ping")'
```

If you remove `.secrets.env` and later need to edit the password again:

```bash
AGE_IDENTITY_FILE=./age-key.txt ../../envoyage decrypt
```

Use `--force` only when you intentionally want to replace an existing
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

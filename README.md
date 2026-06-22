# Envoyage

Envoyage is a lightweight encrypted environment-variable loader for Docker Compose.
It decrypts `.env.age` files in memory, parses the decrypted dotenv content, and
passes the resulting variables to `docker compose` as child-process environment
variables without writing a plaintext `.env` file to disk.

Envoyage exists for the space between plain `.env` files and full secret
management systems. Docker Compose is convenient, but plaintext secret env files
are easy to leave in Git, backups, shell history, deployment directories, or
shared workspaces. Vault, KMS, and larger platforms are excellent tools, but they
can be too much ceremony for a small Compose deployment.

Envoyage keeps that workflow small:

1. Keep non-secret settings in `.env`.
2. Keep secret source values in `.secrets.env` while editing.
3. Encrypt `.secrets.env` to `.env.age`.
4. Run `envoyage compose ...` so Docker Compose receives the decrypted values
   through its process environment.

This is not meant to be a hard security boundary against root, Docker
administrators, or users who can inspect container environments. Its goal is to
reduce accidental plaintext secret exposure while keeping ordinary Compose usage
simple.

License: MIT.

## Install

```bash
go install github.com/swoogi/envoyage/cmd/envoyage@latest
```

From this repository:

```bash
go build ./cmd/envoyage
./envoyage version
```

The initial Envoyage version is `0.1.0`.

## Create `.env.age`

Envoyage uses the age encryption format through the `filippo.io/age` Go
library. You do not need the `age` CLI for the normal Envoyage workflow.

Create a normal dotenv file for non-secret settings:

```env
DB_HOST=postgres
DB_USER=app
DB_PORT=5432
```

Put only secret values in a separate plaintext source file before encryption:

```env
DB_PASSWORD=secret-password
API_KEY=secret-api-key
```

Generate an age identity with Envoyage:

```bash
sudo mkdir -p /etc/envoyage
sudo envoyage keygen
```

By default, this writes the private identity to
`/etc/envoyage/envoyage-key.txt` and prints the public recipient to stdout. It
creates `/etc/envoyage` when needed, sets host-level permissions, and refuses to
overwrite an existing key unless `--force` is passed:

```bash
sudo envoyage keygen --force
```

When the `docker` group exists, the default permissions are:

```text
/etc/envoyage                  root:docker  750
/etc/envoyage/envoyage-key.txt root:docker  640
```

If the `docker` group does not exist, Envoyage falls back to root-only
permissions for the default identity. Creating the default identity path
requires root; use `--out` for a user-owned custom path.

You can also write a key to a custom path:

```bash
envoyage keygen --out age-key.txt
```

Encrypt the secret env file with an identity:

```bash
envoyage encrypt
```

By default, `envoyage encrypt` reads `.secrets.env`, writes `.env.age`, and uses
`AGE_IDENTITY_FILE` or `/etc/envoyage/envoyage-key.txt` as the identity. You can
pass all paths explicitly when needed:

```bash
envoyage encrypt --in .secrets.env --out .env.age --identity /etc/envoyage/envoyage-key.txt
```

Or encrypt to an explicit recipient, which is useful when the recipient is stored
in documentation, CI configuration, or another secure channel:

```bash
envoyage encrypt --in .secrets.env --out .env.age --recipient age1...
```

By default, `envoyage encrypt` refuses to overwrite an existing output file. Use
`--force` when you intentionally want to replace it:

```bash
envoyage encrypt --in .secrets.env --out .env.age --identity age-key.txt --force
```

To recover or edit the plaintext secret source, decrypt `.env.age` back to
`.secrets.env`:

```bash
envoyage decrypt
```

By default, `envoyage decrypt` reads `.env.age`, writes `.secrets.env`, and uses
the same identity lookup as `encrypt`. It refuses to overwrite `.secrets.env`
unless `--force` is passed:

```bash
envoyage decrypt --force
envoyage decrypt --in .env.age --out .secrets.env --identity age-key.txt
```

### Alternative: use the age CLI directly

Envoyage produces and reads standard age-encrypted files. If you already use the
`age` CLI, you can create `.env.age` outside Envoyage:

```bash
age -r <recipient> -o .env.age .env
```

The resulting `.env.age` can be used the same way:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage compose config
```

## Run

If the identity is stored at the default path, you can run:

```bash
envoyage compose up -d
```

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage compose up -d
```

or pass the identity file explicitly:

```bash
envoyage compose --identity ./age-key.txt config
```

By default, `envoyage compose` automatically loads `.env` and `.env.age` from
the current directory when they exist. `.env` is loaded first, then `.env.age`;
later values override earlier ones.

Explicit env files are still supported. When you pass `--env-file`, Envoyage
uses only the files you provided:

```bash
envoyage compose --env-file .env --env-file .env.age config
```

A practical pattern is to keep non-secret settings in `.env` and encrypted
secrets in `.env.age`. That lets you edit everyday config without decrypting the
secret file:

```bash
envoyage encrypt
envoyage compose up -d
```

Envoyage consumes `--env-file` and `--identity` when provided, then runs Docker
Compose with the remaining arguments:

```bash
envoyage compose --env-file .env.age -f compose.yaml -p myapp up -d
```

executes:

```bash
docker compose -f compose.yaml -p myapp up -d
```

You can override the docker binary with:

```bash
ENVOYAGE_DOCKER_BIN=/usr/bin/docker envoyage compose config
```

## Examples

Runnable examples are available under [`examples/`](./examples/):

- [PostgreSQL + pgAdmin](./examples/postgresql/README.md)
- [MariaDB + Adminer](./examples/mariadb/README.md)
- [Redis](./examples/redis/README.md)
- [MongoDB](./examples/mongodb/README.md)
- [MinIO](./examples/minio/README.md)

Each example includes plaintext non-secret `.env` values, example-only
`.secrets.env` values, and a step-by-step README showing how to encrypt only the
secret file and run Docker Compose through Envoyage.

## Identity File

Envoyage needs an age identity file to decrypt `.age` env files. Lookup order is:

1. `--identity`
2. `AGE_IDENTITY_FILE`
3. `/etc/envoyage/envoyage-key.txt`

Use an explicit identity:

```bash
envoyage compose --identity /path/to/identity.txt up -d
```

or an environment variable:

```bash
AGE_IDENTITY_FILE=/path/to/identity.txt envoyage compose up -d
```

If neither is provided, Envoyage tries `/etc/envoyage/envoyage-key.txt`.
Anyone who can read this file can decrypt matching `.env.age` files. This
default is intended to reduce accidental plaintext env-file exposure, not to
protect against root, Docker administrators, or privileged host users.

## Docker Compose `--env-file`

Docker Compose normally receives `--env-file` directly. With Envoyage, the
wrapper consumes those flags first, reads plaintext dotenv files or decrypts
`.age` files, merges the variables, and passes them through the child process
environment. The consumed `--env-file` flags are not forwarded to real
`docker compose`.

This first version supports:

```bash
envoyage compose up -d
```

It does not yet replace the Docker CLI shape directly:

```bash
docker compose --env-file .env.age up -d
```

## Security Model and Limits

Envoyage is designed to avoid leaving plaintext `.env` files in Git, backups, or
deployment directories. It also helps screen secrets from users who can run the
wrapper but should not casually read plaintext env files.

The intended threat reduction is practical rather than absolute: fewer plaintext
secret files at rest, fewer secrets in project trees, and less casual exposure
when a Compose-based app is copied, backed up, reviewed, or deployed.

Envoyage does not write decrypted dotenv content to disk. It does not print
secret values in its own logs or error messages. Key names may appear in parse
errors, but values are not intentionally included.

The decrypted values do exist in the child process environment. Users with Docker
permissions, root/superuser access, or access to `docker inspect` output may be
able to see container environment variables. Envoyage is not a boundary against
root, superusers, Docker administrators, or host-level process inspection.

The default identity path is `/etc/envoyage/envoyage-key.txt`. Treat it as a
host-level secret. A user who can read that file can decrypt `.env.age` files
encrypted for its recipient.

## Dotenv Support

Supported in the MVP:

```env
KEY=value
KEY="value"
KEY='value'
KEY=value with spaces
# comment
EMPTY=
```

Not supported in the MVP:

```env
MULTILINE="..."
export KEY=value
VAR=${OTHER_VAR}
```

## Not Supported

Envoyage does not provide:

- Central secret management like Vault or KMS
- TTL or revoke semantics
- Protection from Docker-privileged users or root/superuser access
- Automatic handling of `env_file: .env.age` inside `compose.yaml`

## Development

With [go-task](https://taskfile.dev/):

```bash
task check
task build
task test
task coverage
task smoke:crypto
task scan:cve
task sonar
```

`task build` embeds the version from `Taskfile.yml` into the binary.

`task scan:cve` requires Trivy. It uses an installed `govulncheck` binary when
available, otherwise it runs `golang.org/x/vuln/cmd/govulncheck` through
`go run`.

`task sonar` loads Sonar connection settings from `.env` and analysis settings
from `sonar-project.properties`, writes `coverage.out`, then runs
`sonar-scanner`. Both `.env` and `sonar-project.properties` are local-only files
and are ignored by Git. Start from the examples when you want to run Sonar
locally:

```bash
cp .env.example .env
cp sonar-project.example.properties sonar-project.properties
```

## Release

Pushing a semantic version tag builds release archives for Linux, macOS, and
Windows, then publishes them to GitHub Releases with SHA-256 checksums:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow strips the leading `v` and embeds `0.1.0` into the
`envoyage version` output.

Or with Go directly:

```bash
go test ./...
go build ./cmd/envoyage
AGE_IDENTITY_FILE=./age-key.txt ./envoyage compose config
```

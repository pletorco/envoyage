# Envoyage

Pronunciation: `/ˈɛn.vɔɪ.ɪdʒ/`

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

## Requirements

Runtime requirements:

- Docker CLI with the Docker Compose plugin for `envoyage compose`
- An age identity file for decrypting `.env.age`

Build and development requirements:

- Go 1.25 or newer
- [go-task](https://taskfile.dev/) for the documented development tasks
- Trivy and `govulncheck` for `task scan:cve`
- SonarScanner CLI for `task sonar`

Envoyage depends on the age encryption format. The application uses the
`filippo.io/age` Go library directly, so the `age` CLI is not required for the
normal Envoyage commands:

```bash
envoyage keygen
envoyage encrypt
envoyage decrypt
```

Installing the `age` CLI is still useful when you want an independent tool for
interoperability, inspection, or manual encryption outside Envoyage:

```bash
age --version
```

## Install

```bash
go install github.com/swoogi/envoyage/cmd/envoyage@latest
```

From this repository:

```bash
go build ./cmd/envoyage
./envoyage version
```

The current Envoyage version is `0.3.0`.

Install the current binary into a user-local location:

```bash
./envoyage install
export PATH="$HOME/.local/bin:$PATH"
envoyage version
```

By default, `envoyage install` copies the current binary to
`~/.local/lib/envoyage/envoyage` and creates `~/.local/bin/envoyage` as a
symlink. It refuses to overwrite an existing non-Envoyage command symlink. Use
`--force` only when you intentionally want to replace the installed Envoyage
binary and recreate the Envoyage-managed symlink:

```bash
envoyage install --force
envoyage status
envoyage uninstall
hash -r
```

By default, `envoyage uninstall` checks both the user-local install path and the
system-wide install path, then removes only Envoyage-managed symlinks and
binaries. Use `--system`, `--bin-dir`, or `--lib-dir` only when you want to
target one location explicitly. The user-local path is resolved from the current
process user, so `sudo envoyage uninstall` checks root's home plus the
system-wide path. Envoyage-managed `docker` shim symlinks are removed before the
Envoyage binary for the same scope is removed.

For a system-wide install under `/usr/local`, run with elevated privileges:

```bash
sudo ./envoyage install --system
hash -r
envoyage status --system
sudo envoyage uninstall
hash -r
```

System-wide install copies the binary to `/usr/local/lib/envoyage/envoyage` and
creates `/usr/local/bin/envoyage` as a symlink. It follows the same overwrite
rules as the user-local install: existing non-Envoyage commands are not replaced,
and an existing Envoyage install is only replaced with `--force`.

If your current shell still tries an older path such as
`~/.local/bin/envoyage`, refresh the shell command cache with `hash -r` or open a
new shell.

## Shell Completion

Envoyage can generate shell completion scripts:

```bash
envoyage completion bash
envoyage completion zsh
envoyage completion fish
envoyage completion powershell
```

For bash:

```bash
mkdir -p ~/.local/share/bash-completion/completions
envoyage completion bash > ~/.local/share/bash-completion/completions/envoyage
```

For zsh:

```bash
mkdir -p ~/.zfunc
envoyage completion zsh > ~/.zfunc/_envoyage
echo 'fpath=(~/.zfunc $fpath)' >> ~/.zshrc
echo 'autoload -Uz compinit && compinit' >> ~/.zshrc
```

For fish:

```bash
mkdir -p ~/.config/fish/completions
envoyage completion fish > ~/.config/fish/completions/envoyage.fish
```

For PowerShell:

```powershell
envoyage completion powershell >> $PROFILE
```

## Quickstart

This walkthrough uses the PostgreSQL example. It keeps non-secret settings in
`.env`, encrypts example-only passwords from `.secrets.env` into `.env.age`,
then runs Docker Compose through Envoyage.

Build Envoyage from the repository root:

```bash
go build -o envoyage ./cmd/envoyage
./envoyage install
export PATH="$HOME/.local/bin:$PATH"
```

Move into the example:

```bash
cd examples/postgresql
```

Generate a local age identity for the example:

```bash
envoyage keygen --out age-key.txt
```

Encrypt `.secrets.env` to `.env.age`:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage encrypt
```

Preview the Compose configuration:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage compose -f compose.yaml config
```

Start the services:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage compose -f compose.yaml up -d
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

The committed `.secrets.env` in `examples/` is intentionally included for a
copy-and-run demo. In real projects, do not commit plaintext `.secrets.env`;
keep `.env.age` and the age identity distribution under your own deployment
policy.

Optional Docker-shaped flow:

```bash
envoyage shim install --bin-dir ./bin
PATH="$PWD/bin:$PATH" ENVOYAGE_DOCKER_BIN=/usr/bin/docker docker compose -f compose.yaml config
envoyage shim uninstall --bin-dir ./bin
```

## Extract And Inline

Envoyage can help migrate Compose files that keep fixed values directly under
`services.*.environment`.

Preview an extraction:

```bash
envoyage env extract
```

This scans `compose.yaml`, `compose.yml`, `docker-compose.yaml`, or
`docker-compose.yml` when exactly one exists. Pass `--compose` when you want a
specific file:

```bash
envoyage env extract --compose compose.yaml
```

The preview prints key names only. Values are not printed:

```text
extract: compose.yaml

.env:
  DB_HOST
  DB_USER

.secrets.env:
  DB_PASSWORD
  API_TOKEN

compose updates:
  services.app.environment.DB_HOST
  services.app.environment.DB_PASSWORD

dry-run: pass --write to update files
```

Write `.env`, `.secrets.env`, and update the Compose file:

```bash
envoyage env extract --write
```

Secret-looking keys such as `PASSWORD`, `SECRET`, `TOKEN`, `API_KEY`,
`PRIVATE_KEY`, `ACCESS_KEY`, and `CREDENTIAL` are written to `.secrets.env`.
Other extracted keys are written to `.env`. Existing keys are reused when their
values match; conflicts stop the command without printing the conflicting
values. Disable secret splitting when you want every extracted key in `.env`:

```bash
envoyage env extract --secrets=false --write
```

The inverse operation is `inline`. It writes a separate Compose file with
`${VAR}` values replaced from `.env`, `.secrets.env`, and `.env.age`:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage env inline --out compose.inline.yaml
```

`inline` never modifies the source Compose file and requires `--out`. The output
file may contain plaintext secrets, so treat it as sensitive and do not commit
it. Existing output files are not overwritten unless `--force` is passed:

```bash
AGE_IDENTITY_FILE=./age-key.txt envoyage env inline --out compose.inline.yaml --force
```

## Create `.env.age`

Envoyage uses the age encryption format through the `filippo.io/age` Go
library. You do not need the `age` CLI for the normal Envoyage workflow, but
files produced by Envoyage are standard age-encrypted files.

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

The default command shape is:

```bash
envoyage compose up -d
```

## Optional Docker/Podman Shim Mode

Envoyage can also run as an optional Docker or Podman shim. This mode is
disabled unless the Envoyage binary is executed with the name `docker` or
`podman`, usually through a symlink placed earlier in `PATH` than the real
runtime CLI.

In shim mode, only `docker compose ...` and `podman compose ...` are intercepted
by Envoyage. Other runtime commands are passed through to the real binary:

```bash
docker ps
docker version
podman ps
podman version
```

Create user-local shims for the runtimes detected on `PATH`:

```bash
envoyage shim install
```

`envoyage shim install` first ensures Envoyage itself is installed to a stable
location, then creates `docker` and/or `podman` shim symlinks to that installed
binary. The default `--runtime auto` mode creates shims only for runtimes that
are detected. You can be explicit when needed:

```bash
envoyage shim install --runtime docker
envoyage shim install --runtime podman
envoyage shim install --runtime all
```

For the default user-local mode, each shim points to
`~/.local/lib/envoyage/envoyage`. With `--system`, shims point to
`/usr/local/lib/envoyage/envoyage`. Use `--force` when you intentionally want to
refresh both the installed Envoyage binary and the Envoyage-managed shim
symlinks.

Put `~/.local/bin` before the real runtime directory in `PATH`, and optionally
point Envoyage at the real runtime binaries:

```bash
export PATH="$HOME/.local/bin:$PATH"
export ENVOYAGE_DOCKER_BIN=/usr/bin/docker
export ENVOYAGE_PODMAN_BIN=/usr/bin/podman
```

Check the current shim state:

```bash
envoyage shim status
```

Then Compose can be run with the Docker-shaped command:

```bash
docker compose --env-file .env.age up -d
```

Or with Podman when `podman compose` is available:

```bash
podman compose --env-file .env.age up -d
```

If `ENVOYAGE_DOCKER_BIN` or `ENVOYAGE_PODMAN_BIN` is not set, shim mode searches
`PATH` for the next executable named `docker` or `podman` that is not the
Envoyage shim itself. Setting the runtime binary explicitly is recommended
because it avoids ambiguity.

Remove the shim when you no longer want Docker-shaped interception:

```bash
envoyage shim uninstall
hash -r
```

By default, `envoyage shim uninstall` checks both the user-local shim path and
the system-wide shim path for both Docker and Podman, then removes only
Envoyage-managed shim symlinks. Use `--runtime docker|podman|all`, `--system`,
or `--bin-dir` only when you want to target one runtime or location explicitly.
The user-local path is resolved from the current process user, so `sudo envoyage
shim uninstall` checks root's home plus the system-wide path.

For a system-wide shim under `/usr/local/bin`, run:

```bash
sudo envoyage shim install --system
hash -r
envoyage shim status --system
sudo envoyage shim uninstall
hash -r
```

System-wide shim mode first installs Envoyage under `/usr/local/lib/envoyage`,
then creates `/usr/local/bin/docker` and/or `/usr/local/bin/podman` as symlinks
to that installed binary. It still refuses to overwrite existing non-Envoyage
runtime files, even with `--force`.

`envoyage shim install` refuses to overwrite an existing non-Envoyage runtime
file, even with `--force`. `--force` only recreates shim symlinks that already
point at Envoyage.

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
task test:e2e
task coverage
task smoke:crypto
task scan:cve
task sonar
```

`task build` embeds the version from `Taskfile.yml` into the binary.

`task test:e2e` builds a temporary Envoyage binary and runs end-to-end tests
against fake Docker and Podman runtime scripts. It does not require a Docker or
Podman daemon.

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

Or with Go directly:

```bash
go test ./...
go build ./cmd/envoyage
AGE_IDENTITY_FILE=./age-key.txt ./envoyage compose config
```

## Release

Releases are built by GitHub Actions from semantic version tags.

Before tagging, make sure the tree is clean and the checks pass:

```bash
task check
git status --short
```

Create and push a tag:

```bash
git tag v0.3.0
git push origin v0.3.0
```

Pushing the tag runs the release workflow. It builds archives for Linux, macOS,
and Windows on `amd64` and `arm64`, writes `checksums.txt`, and publishes the
files to GitHub Releases.

The workflow strips the leading `v` and embeds the tag version into
`envoyage version`. For example, `v0.3.0` produces:

```text
envoyage 0.3.0
```

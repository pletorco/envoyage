# Changelog

All notable changes to Envoyage are documented in this file.

## 0.2.1

### Added

- Added `envoyage install`, `envoyage status`, and `envoyage uninstall` for
  user-local Envoyage binary installation.
- Added `envoyage completion bash|zsh|fish|powershell` for shell completion
  script generation.
- Added system-wide install mode with `envoyage install --system` and
  `envoyage shim install --system`.

### Changed

- Bumped the development version to `0.2.1`.
- Changed `envoyage uninstall` and `envoyage shim uninstall` to automatically
  check both default user-local and system-wide locations when no explicit
  location flag is passed.
- Changed `envoyage uninstall` to remove Envoyage-managed `docker` shims before
  removing Envoyage binaries.
- Changed `envoyage shim install` to install Envoyage first and point the
  `docker` shim at the stable installed binary.

### Security

- Updated `golang.org/x/crypto` and `golang.org/x/sys` to patched versions.

## 0.2.0

### Added

- Added optional Docker shim mode for `docker compose ...`.
- Added `envoyage shim status`, `envoyage shim install`, and
  `envoyage shim uninstall`.
- Added shim smoke testing through `task smoke:shim`.

### Changed

- Bumped the development version to `0.2.0`.

## 0.1.0

### Added

- Initial Envoyage MVP.
- Added `envoyage compose` for loading plaintext `.env` and encrypted
  `.env.age` files into Docker Compose child-process environments.
- Added `envoyage keygen`, `envoyage encrypt`, and `envoyage decrypt`.
- Added examples for PostgreSQL, MariaDB, Redis, MongoDB, and MinIO.
- Added GitHub CI and release archive automation.

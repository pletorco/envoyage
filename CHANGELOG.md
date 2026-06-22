# Changelog

All notable changes to Envoyage are documented in this file.

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

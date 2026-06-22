# Envoyage Examples

These examples show the intended Envoyage workflow with Docker Compose:

1. Keep non-secret configuration in plaintext `.env`.
2. Put only secret values in `.secrets.env`.
3. Generate an age identity with Envoyage.
4. Encrypt `.secrets.env` to `.env.age`.
5. Run `envoyage compose ...`; Envoyage automatically loads `.env` and `.env.age`.
6. Stop distributing the plaintext `.secrets.env` file in real projects.

This split keeps everyday non-secret changes editable without decrypting the
secret file.

If you need to edit secret values later, recover the plaintext source with:

```bash
envoyage decrypt
```

Envoyage refuses to overwrite `.secrets.env` unless you pass `--force`.

The committed `.env` and `.secrets.env` files in this directory contain
example-only values. They are intentionally included so you can follow the
workflow end to end. Do not reuse these values in real environments.

For real projects, a practical pattern is:

1. Commit `.env`.
2. Do not commit `.secrets.env`.
3. Generate an age identity with Envoyage.
4. Commit `.env.age` if your deployment workflow expects encrypted secrets in
   Git, or distribute it through your deployment artifact flow.

Available examples:

- [PostgreSQL](./postgresql/README.md)
- [MariaDB](./mariadb/README.md)
- [Redis](./redis/README.md)
- [MongoDB](./mongodb/README.md)
- [MinIO](./minio/README.md)

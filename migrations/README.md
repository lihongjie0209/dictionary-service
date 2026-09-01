# Migrations

Database-specific migrations live under `mysql`, `postgres`, and `kingbase`. Set `migration.path` to the matching directory, for example `migrations/postgres`. Review indexes, collation and online-DDL impact against production data before deployment.

Migration `000004` preserves existing rows as platform or tenant defaults with an empty `application_id`. Application dictionaries use a non-empty application ID and resolve ahead of tenant and platform defaults.

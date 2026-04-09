-- +migrate Up
alter table if exists users
  add column if not exists department text;

alter table if exists users
  add column if not exists position text;

alter table if exists users
  add column if not exists corporate_email text;

create index if not exists idx_users_corporate_email_lower
  on users (lower(corporate_email))
  where corporate_email is not null and btrim(corporate_email) <> '';

-- +migrate Down
-- Rollback is intentionally not supported.

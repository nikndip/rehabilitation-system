-- +migrate Up
create extension if not exists "uuid-ossp";

-- Compatibility migration for databases created from older snapshots.
-- Ensures all tables/columns required by current nutrition points flow exist.

alter table if exists user_points
  add column if not exists points_total int not null default 0;

create table if not exists nutrition_reminder_settings (
  user_id uuid primary key references users(id) on delete cascade,
  meal_reminder_lead_minutes int not null default 20,
  meal_sla_minutes int not null default 60,
  hydration_1030_enabled boolean not null default true,
  hydration_1500_enabled boolean not null default true,
  hydration_1800_enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (meal_reminder_lead_minutes >= 0 and meal_reminder_lead_minutes <= 240),
  check (meal_sla_minutes >= 15 and meal_sla_minutes <= 360)
);

alter table if exists nutrition_plan_meals
  add column if not exists status text not null default 'planned';
alter table if exists nutrition_plan_meals
  add column if not exists planned_time text not null default '';
alter table if exists nutrition_plan_meals
  add column if not exists smart_swap_from_meal_id text;
alter table if exists nutrition_plan_meals
  add column if not exists completed_at timestamptz;
alter table if exists nutrition_plan_meals
  add column if not exists skipped_at timestamptz;

alter table if exists nutrition_day_progress
  add column if not exists points_awarded boolean not null default false;
alter table if exists nutrition_day_progress
  add column if not exists completed_at timestamptz;

alter table if exists nutrition_hydration_logs
  add column if not exists status text not null default 'planned';
alter table if exists nutrition_hydration_logs
  add column if not exists completed_at timestamptz;
alter table if exists nutrition_hydration_logs
  add column if not exists updated_at timestamptz not null default now();

create table if not exists nutrition_points_ledger (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  change_amount int not null,
  balance_after int,
  reason_code text not null,
  reason text not null default '',
  source_type text not null default 'system',
  source_id text,
  created_by uuid references users(id) on delete set null,
  created_at timestamptz not null default now()
);

alter table if exists nutrition_points_ledger
  add column if not exists change_amount int;
alter table if exists nutrition_points_ledger
  add column if not exists balance_after int;
alter table if exists nutrition_points_ledger
  add column if not exists reason_code text;
alter table if exists nutrition_points_ledger
  add column if not exists reason text not null default '';
alter table if exists nutrition_points_ledger
  add column if not exists source_type text not null default 'system';
alter table if exists nutrition_points_ledger
  add column if not exists source_id text;
alter table if exists nutrition_points_ledger
  add column if not exists created_by uuid references users(id) on delete set null;
alter table if exists nutrition_points_ledger
  add column if not exists created_at timestamptz not null default now();

create index if not exists idx_nutrition_points_ledger_user_created
  on nutrition_points_ledger(user_id, created_at desc);

create table if not exists nutrition_day_event_history (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  event_type text not null,
  slot_key text,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

alter table if exists nutrition_day_event_history
  add column if not exists metadata jsonb not null default '{}'::jsonb;
alter table if exists nutrition_day_event_history
  add column if not exists created_at timestamptz not null default now();

create index if not exists idx_nutrition_day_event_history_user_day
  on nutrition_day_event_history(user_id, day_date desc, created_at desc);

-- +migrate Down
-- Rollback is intentionally not supported.

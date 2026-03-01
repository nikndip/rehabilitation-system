-- +migrate Up
create table if not exists nutrition_questionnaire_responses (
  user_id uuid primary key references users(id) on delete cascade,
  answers jsonb not null default '{}',
  updated_at timestamptz not null default now()
);

create table if not exists nutrition_hydration_logs (
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  reminder_key text not null,
  status text not null default 'planned',
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (user_id, day_date, reminder_key)
);

create index if not exists idx_nutrition_hydration_logs_user_date
  on nutrition_hydration_logs(user_id, day_date desc);

-- +migrate Down
drop index if exists idx_nutrition_hydration_logs_user_date;
drop table if exists nutrition_hydration_logs;
drop table if exists nutrition_questionnaire_responses;

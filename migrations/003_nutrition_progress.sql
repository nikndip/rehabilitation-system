-- +migrate Up
alter table nutrition_plan_meals add column if not exists status text not null default 'planned';
alter table nutrition_plan_meals add column if not exists planned_time text not null default '';
alter table nutrition_plan_meals add column if not exists smart_swap_from_meal_id text;
alter table nutrition_plan_meals add column if not exists completed_at timestamptz;
alter table nutrition_plan_meals add column if not exists skipped_at timestamptz;

create index if not exists idx_nutrition_plan_meals_user_day on nutrition_plan_meals(user_id, day_key);

create table if not exists nutrition_day_progress (
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  completed_slots int not null default 0,
  total_slots int not null default 4,
  day_completed boolean not null default false,
  points_awarded boolean not null default false,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (user_id, day_date)
);

create index if not exists idx_nutrition_day_progress_user_date on nutrition_day_progress(user_id, day_date desc);

create table if not exists nutrition_user_stats (
  user_id uuid primary key references users(id) on delete cascade,
  current_streak int not null default 0,
  best_streak int not null default 0,
  total_completed_days int not null default 0,
  last_completed_day date,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists nutrition_reward_redemptions (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  reward_id text not null,
  reward_title text not null,
  points_cost int not null default 0,
  status text not null default 'issued',
  redeemed_at timestamptz not null default now(),
  used_at timestamptz
);

create index if not exists idx_nutrition_reward_redemptions_user on nutrition_reward_redemptions(user_id, redeemed_at desc);

create table if not exists nutrition_events (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  message text not null,
  created_at timestamptz not null default now()
);

create index if not exists idx_nutrition_events_user on nutrition_events(user_id, created_at desc);

-- +migrate Down
drop table if exists nutrition_events;
drop table if exists nutrition_reward_redemptions;
drop table if exists nutrition_user_stats;
drop table if exists nutrition_day_progress;

drop index if exists idx_nutrition_plan_meals_user_day;
alter table nutrition_plan_meals drop column if exists skipped_at;
alter table nutrition_plan_meals drop column if exists completed_at;
alter table nutrition_plan_meals drop column if exists smart_swap_from_meal_id;
alter table nutrition_plan_meals drop column if exists planned_time;
alter table nutrition_plan_meals drop column if exists status;

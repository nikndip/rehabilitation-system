-- +migrate Up
create table if not exists nutrition_plan_meals (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  day_key text not null,
  meal_slot text not null,
  meal_id text not null,
  meal_name text not null,
  calories int not null default 0,
  protein int not null default 0,
  carbs int not null default 0,
  fats int not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (user_id, day_key, meal_slot)
);

-- +migrate Down
drop table if exists nutrition_plan_meals;

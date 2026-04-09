-- +migrate Up
-- Stores meal assignments by calendar date to avoid collisions between weeks.

alter table if exists nutrition_plan_meals
  add column if not exists day_date date;

with current_week as (
  select (current_date - ((extract(isodow from current_date)::int) - 1))::date as week_start
)
update nutrition_plan_meals npm
set day_date = current_week.week_start + case lower(trim(npm.day_key))
  when 'monday' then 0
  when 'tuesday' then 1
  when 'wednesday' then 2
  when 'thursday' then 3
  when 'friday' then 4
  when 'saturday' then 5
  when 'sunday' then 6
  else 0
end
from current_week
where npm.day_date is null;

update nutrition_plan_meals
set day_date = current_date
where day_date is null;

alter table if exists nutrition_plan_meals
  alter column day_date set not null;

do $$
declare
  old_constraint text;
begin
  if to_regclass('nutrition_plan_meals') is null then
    return;
  end if;

  for old_constraint in
    select c.conname
    from pg_constraint c
    where c.conrelid = 'nutrition_plan_meals'::regclass
      and c.contype = 'u'
      and lower(pg_get_constraintdef(c.oid)) like '%user_id%day_key%meal_slot%'
  loop
    execute format('alter table nutrition_plan_meals drop constraint %I', old_constraint);
  end loop;

  if not exists (
    select 1
    from pg_constraint c
    where c.conrelid = 'nutrition_plan_meals'::regclass
      and c.contype = 'u'
      and lower(pg_get_constraintdef(c.oid)) like '%user_id%day_date%meal_slot%'
  ) then
    alter table nutrition_plan_meals
      add constraint nutrition_plan_meals_user_day_date_meal_slot_key unique (user_id, day_date, meal_slot);
  end if;
end $$;

drop index if exists idx_nutrition_plan_meals_user_day;
create index if not exists idx_nutrition_plan_meals_user_day
  on nutrition_plan_meals(user_id, day_date);

-- +migrate Down
-- Rollback is intentionally not supported.

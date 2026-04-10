-- +migrate Up
-- Additional compatibility for legacy support schemas where extra columns may block inserts.

do $$
declare
  col record;
begin
  if to_regclass('support_tickets') is not null then
    for col in
      select c.column_name
      from information_schema.columns c
      where c.table_schema = 'public'
        and c.table_name = 'support_tickets'
        and c.is_nullable = 'NO'
        and c.column_default is null
        and c.column_name not in ('id', 'user_id', 'subject', 'status', 'created_at', 'updated_at', 'last_message_at')
    loop
      execute format('alter table support_tickets alter column %I drop not null', col.column_name);
    end loop;
  end if;

  if to_regclass('support_ticket_messages') is not null then
    for col in
      select c.column_name
      from information_schema.columns c
      where c.table_schema = 'public'
        and c.table_name = 'support_ticket_messages'
        and c.is_nullable = 'NO'
        and c.column_default is null
        and c.column_name not in ('id', 'ticket_id', 'sender_id', 'sender_role', 'message', 'created_at')
    loop
      execute format('alter table support_ticket_messages alter column %I drop not null', col.column_name);
    end loop;
  end if;
end $$;

-- +migrate Down
-- Rollback is intentionally not supported.

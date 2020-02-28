do $$
begin

create table if not exists app_metrics(
  app text not null,
  metric text not null,
  created_at timestamptz not null default now(),
  deleted boolean not null default false,
  constraint PK primary key (app, metric)
);

end
$$;
create table schema_version(
  version integer primary key,
  description text not null,
  is_current boolean default 0 check (is_current in (0, 1))
);

-- make sure there's only one active version
create unique index schema_version_uk on schema_version(is_current)
  where is_current = 1;

-- Insert current schema version. This should be increased every time changes are made
-- to schema.sql and migration.sql
insert into schema_version(version, description, is_current)
  values(1, 'initial schema', 1);

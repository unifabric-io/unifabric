CREATE TABLE IF NOT EXISTS {{migrations_table}} (
  version UInt64,
  name String,
  applied_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(applied_at)
ORDER BY version

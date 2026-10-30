-- Every daily snapshot of snip's links table: one row per link per snapshot day.

select
    cast(slug as varchar)          as slug,
    cast(url as varchar)           as url,
    cast(owner_id as bigint)       as owner_id,
    cast(clicks as bigint)         as click_counter,   -- snip's own running count, for reconciliation
    cast(created_at as timestamptz) as created_at,
    cast(dt as date)               as snapshot_date
from {{ source('lake', 'links') }}

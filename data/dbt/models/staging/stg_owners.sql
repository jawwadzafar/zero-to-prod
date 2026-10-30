-- Every daily snapshot of snip's API key owners.

select
    cast(owner_id as bigint)        as owner_id,
    cast(name as varchar)           as owner_name,
    cast(created_at as timestamptz) as created_at,
    cast(revoked_at as timestamptz) as revoked_at,
    cast(dt as date)                as snapshot_date
from {{ source('lake', 'owners') }}

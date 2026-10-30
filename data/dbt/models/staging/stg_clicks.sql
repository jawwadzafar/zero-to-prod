-- One row per click event, duplicates removed.
-- The stream delivers at least once, so the same event_id can arrive twice;
-- keeping one row per event_id makes every model downstream count it once.

with source as (

    select * from {{ source('lake', 'clicks') }}

),

typed as (

    select
        cast(event_id as varchar)       as event_id,
        cast(slug as varchar)           as slug,
        cast(clicked_at as timestamptz) as clicked_at,
        cast(dt as date)                as partition_date
    from source

)

select
    event_id,
    slug,
    clicked_at,
    cast(clicked_at as date) as click_date,   -- the UTC day (the profile sets TimeZone = UTC)
    partition_date
from typed
qualify row_number() over (partition by event_id order by partition_date, clicked_at) = 1

-- snip's audit trail: who created and deleted which link, and when.
-- detail is JSON text; the URL is pulled out into a column.

select
    cast(id as bigint)                          as audit_event_id,
    cast("at" as timestamptz)                   as occurred_at,
    cast(owner_id as bigint)                    as owner_id,
    cast(action as varchar)                     as action,
    cast(slug as varchar)                       as slug,
    json_extract_string(detail, '$.url')        as url,
    cast(request_id as varchar)                 as request_id
from {{ source('lake', 'audit_events') }}

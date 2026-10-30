-- dim_links: one row per link snip has ever had, live or deleted.
-- Grain: one row per slug.
--
-- Clicks can point at a link that was deleted before the latest snapshot, or
-- created and deleted between two snapshots. The audit trail knows about
-- those, so the dimension takes every slug from either place: a fact row
-- should never point at a link the dimension doesn't have.
-- (If a deleted slug is created again, this keeps the latest version only;
-- chapter 17.4 calls that a type 1 slowly changing dimension.)

with snapshots as (

    select * from {{ ref('stg_links') }}

),

latest_snapshot as (

    select max(snapshot_date) as snapshot_date from snapshots

),

latest_per_slug as (   -- the newest snapshot row of each slug

    select *
    from snapshots
    qualify row_number() over (partition by slug order by snapshot_date desc) = 1

),

audit as (

    select * from {{ ref('stg_audit_events') }}

),

created as (           -- the latest creation of each slug

    select slug, url, owner_id, occurred_at as created_at
    from audit
    where action = 'link.created'
    qualify row_number() over (partition by slug order by occurred_at desc) = 1

),

deleted as (

    select slug, max(occurred_at) as deleted_at
    from audit
    where action = 'link.deleted'
    group by slug

),

owners as (            -- each owner's name from the newest snapshot

    select owner_id, owner_name
    from {{ ref('stg_owners') }}
    qualify row_number() over (partition by owner_id order by snapshot_date desc) = 1

),

slugs as (

    select slug from latest_per_slug
    union
    select slug from created

),

joined as (

    select
        slugs.slug,
        coalesce(s.url, c.url)               as url,
        coalesce(s.owner_id, c.owner_id)     as owner_id,
        coalesce(s.created_at, c.created_at) as created_at,
        s.click_counter                      as last_click_counter,
        s.snapshot_date                      as last_seen_in_snapshot,
        d.deleted_at
    from slugs
    left join latest_per_slug as s on s.slug = slugs.slug
    left join created         as c on c.slug = slugs.slug
    left join deleted         as d on d.slug = slugs.slug

)

select
    joined.slug,
    joined.url,
    regexp_extract(joined.url, '^[a-z]+://([^/:?#]+)', 1) as url_host,
    joined.owner_id,
    owners.owner_name,
    joined.created_at,
    cast(joined.created_at as date)                       as created_date,
    -- a deletion only counts if it came after the (latest) creation
    case when joined.deleted_at >= joined.created_at then joined.deleted_at end as deleted_at,
    (
        joined.last_seen_in_snapshot is distinct from latest_snapshot.snapshot_date
        or coalesce(joined.deleted_at >= joined.created_at, false)
    )                                                     as is_deleted,
    joined.last_click_counter,
    joined.last_seen_in_snapshot
from joined
cross join latest_snapshot
left join owners on owners.owner_id = joined.owner_id

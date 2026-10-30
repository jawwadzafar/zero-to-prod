-- link_clicks_daily: clicks per link per UTC day.
-- Grain: one row per (slug, click_date) that had at least one click.
-- Days without clicks have no row: sum, don't count rows, when you add up a range.

select
    f.slug,
    f.click_date,
    l.owner_id,
    count(*)          as clicks,
    min(f.clicked_at) as first_click_at,
    max(f.clicked_at) as last_click_at
from {{ ref('fct_clicks') }} as f
left join {{ ref('dim_links') }} as l on l.slug = f.slug
group by all

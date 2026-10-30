-- A generic test: fails on every row whose column is outside [min_value, max_value].
-- Either bound may be left out. The bounds are SQL expressions, so
-- "current_date + 1" or "date '2020-01-01'" both work.
-- (The dbt_utils package has one like it; writing it here keeps the project
-- free of package downloads, and shows that a test is a query for bad rows.)

{% test accepted_range(model, column_name, min_value=none, max_value=none) %}

select *
from {{ model }}
where false
{%- if min_value is not none %}
    or {{ column_name }} < {{ min_value }}
{%- endif %}
{%- if max_value is not none %}
    or {{ column_name }} > {{ max_value }}
{%- endif %}

{% endtest %}

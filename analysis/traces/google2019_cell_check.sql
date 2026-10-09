#standardSQL
# Wattproof: check query for google2019_cell.sql. Same joins and tiering, on a 1% block sample of
# instance_usage and instance_events, returning only tier totals per 5 minutes. Sampled tables
# cannot be referenced twice, so the per-service and power-domain outputs are left out.
# Method follows Google's analysis notebooks: tasks inside alloc sets are excluded (their alloc
# instances carry the reservation); tiers by priority. Requests are the limits in force at the
# time of each usage window (they change, for example under Autopilot).
# Time: microseconds since 600 s before 2019-05-01 00:00 PT; buckets are aligned to PT clock time.

WITH
coll AS (
  SELECT collection_id,
    MAX(priority) AS priority,
    MAX(scheduling_class) AS sched_class,
    MAX(vertical_scaling) AS vscale,
    MAX(collection_type) AS ctype
  FROM `google.com:google-cluster-data.clusterdata_2019_a.collection_events`
  GROUP BY collection_id
),
req AS (
  # Each event's request holds until the instance's next event.
  SELECT collection_id, instance_index, time AS t0,
    LEAD(time) OVER (PARTITION BY collection_id, instance_index ORDER BY time) AS t1,
    resource_request.cpus AS req_cpu,
    resource_request.memory AS req_mem
  FROM `google.com:google-cluster-data.clusterdata_2019_a.instance_events` TABLESAMPLE SYSTEM (1 PERCENT)
  WHERE (alloc_collection_id IS NULL OR alloc_collection_id = 0)
    AND resource_request.cpus IS NOT NULL
),
win AS (
  SELECT u.collection_id, u.instance_index, u.machine_id,
    CAST(FLOOR((u.start_time - 6e8) / 3e8) AS INT64) AS b5,
    (u.end_time - u.start_time) / 3e8 AS w,       # window length in 5-minute units
    u.average_usage.cpus AS cpu,
    u.average_usage.memory AS mem,
    r.req_cpu, r.req_mem
  FROM `google.com:google-cluster-data.clusterdata_2019_a.instance_usage` AS u TABLESAMPLE SYSTEM (1 PERCENT)
  JOIN req AS r
    ON u.collection_id = r.collection_id
   AND u.instance_index = r.instance_index
   AND u.start_time >= r.t0
   AND (r.t1 IS NULL OR u.start_time < r.t1)
  WHERE u.end_time > u.start_time
),
tagged AS (
  SELECT win.*, c.priority, c.sched_class, c.vscale, c.ctype,
    CASE
      WHEN c.priority BETWEEN 0 AND 99 THEN 'free'
      WHEN c.priority BETWEEN 100 AND 115 THEN 'beb'
      WHEN c.priority BETWEEN 116 AND 119 THEN 'mid'
      ELSE 'prod'
    END AS tier
  FROM win JOIN coll AS c USING (collection_id)
)
SELECT 'tier', tier, b5,
  SUM(cpu * w), NULL, SUM(req_cpu * w), SUM(mem * w), SUM(req_mem * w), COUNT(*),
  NULL, NULL, NULL, NULL
FROM tagged GROUP BY tier, b5

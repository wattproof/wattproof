#standardSQL
# Wattproof: inputs for the simulator from one cell of the Google 2019 trace.
# One scan of the large tables. Three outputs in one result, told apart by `kind`:
#   svc   per mid- or production-tier collection alive for at least a week, per hour: CPU and memory usage and
#         requests (time-weighted means), the highest 5-minute CPU usage in the hour, attributes
#   tier  per tier and 5 minutes: CPU and memory usage and requests (validation against the paper)
#   pdu   per power domain and 5 minutes: CPU usage, all tiers and production tier (power analysis)
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
  FROM `google.com:google-cluster-data.clusterdata_2019_a.instance_events`
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
  FROM `google.com:google-cluster-data.clusterdata_2019_a.instance_usage` AS u
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
),
svc5 AS (
  SELECT collection_id, b5,
    SUM(cpu * w) AS cpu_use, SUM(req_cpu * w) AS cpu_req,
    SUM(mem * w) AS mem_use, SUM(req_mem * w) AS mem_req,
    COUNT(DISTINCT instance_index) AS n
  FROM tagged
  WHERE priority >= 116
  GROUP BY collection_id, b5
),
svc AS (
  SELECT collection_id, DIV(b5, 12) AS h,
    SUM(cpu_use) / 12 AS cpu_use, MAX(cpu_use) AS cpu_use_max5, SUM(cpu_req) / 12 AS cpu_req,
    SUM(mem_use) / 12 AS mem_use, SUM(mem_req) / 12 AS mem_req, MAX(n) AS n
  FROM svc5
  WHERE b5 >= 0
  GROUP BY collection_id, h
),
svc_long AS (
  SELECT * FROM svc
  WHERE collection_id IN (SELECT collection_id FROM svc GROUP BY collection_id HAVING COUNT(*) >= 168)
)
SELECT 'svc' AS kind, CAST(s.collection_id AS STRING) AS k, s.h AS t,
  s.cpu_use, s.cpu_use_max5, s.cpu_req, s.mem_use, s.mem_req, s.n,
  c.priority, c.sched_class, c.vscale, c.ctype
FROM svc_long AS s JOIN coll AS c USING (collection_id)
UNION ALL
SELECT 'tier', tier, b5,
  SUM(cpu * w), NULL, SUM(req_cpu * w), SUM(mem * w), SUM(req_mem * w), COUNT(*),
  NULL, NULL, NULL, NULL
FROM tagged GROUP BY tier, b5
UNION ALL
SELECT 'pdu', CONCAT(m.pdu, IF(t.tier = 'prod', ':prod', ':other')), t.b5,
  SUM(t.cpu * t.w), NULL, SUM(t.req_cpu * t.w), NULL, NULL, COUNT(DISTINCT t.machine_id),
  NULL, NULL, NULL, NULL
FROM tagged AS t
JOIN `google.com:google-cluster-data.powerdata_2019.machine_to_pdu_mapping` AS m
  ON m.machine_id = t.machine_id AND m.cell = 'a'
GROUP BY 2, t.b5

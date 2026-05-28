CREATE TABLE IF NOT EXISTS default.flows_raw (
  `type` Int32,
  `time_received` DateTime64(9),
  `sequence_num` UInt32,
  `sampling_rate` UInt64,
  `sampler_address` FixedString(16),
  `time_flow_start` DateTime64(9),
  `time_flow_end` DateTime64(9),
  `bytes` UInt64,
  `packets` UInt64,
  `src_addr` FixedString(16),
  `dst_addr` FixedString(16),
  `src_as` UInt32,
  `dst_as` UInt32,
  `etype` UInt32,
  `proto` UInt32,
  `src_port` UInt32,
  `dst_port` UInt32,
  `src_k8s_pod_name` String DEFAULT '',
  `src_k8s_pod_namespace` LowCardinality(String) DEFAULT '',
  `src_k8s_node_name` LowCardinality(String) DEFAULT '',
  `dst_k8s_pod_name` String DEFAULT '',
  `dst_k8s_pod_namespace` LowCardinality(String) DEFAULT '',
  `dst_k8s_node_name` LowCardinality(String) DEFAULT '',
  `src_k8s_top_owner_kind` LowCardinality(String) DEFAULT '',
  `src_k8s_top_owner_name` String DEFAULT '',
  `src_k8s_top_owner_namespace` LowCardinality(String) DEFAULT '',
  `dst_k8s_top_owner_kind` LowCardinality(String) DEFAULT '',
  `dst_k8s_top_owner_name` String DEFAULT '',
  `dst_k8s_top_owner_namespace` LowCardinality(String) DEFAULT '',
  INDEX idx_src_pod src_k8s_pod_name TYPE bloom_filter(0.01) GRANULARITY 4,
  INDEX idx_dst_pod dst_k8s_pod_name TYPE bloom_filter(0.01) GRANULARITY 4,
  INDEX idx_src_owner src_k8s_top_owner_name TYPE bloom_filter(0.01) GRANULARITY 4,
  INDEX idx_dst_owner dst_k8s_top_owner_name TYPE bloom_filter(0.01) GRANULARITY 4,
  INDEX idx_src_node src_k8s_node_name TYPE set(10000) GRANULARITY 4,
  INDEX idx_dst_node dst_k8s_node_name TYPE set(10000) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toDate(time_flow_start)
ORDER BY (time_flow_start, src_k8s_pod_namespace, src_k8s_top_owner_kind, src_k8s_top_owner_name, src_k8s_pod_name, dst_k8s_pod_namespace, dst_k8s_top_owner_kind, dst_k8s_top_owner_name, dst_k8s_pod_name, src_k8s_node_name, dst_k8s_node_name, src_addr, dst_addr)
TTL time_flow_start + toIntervalDay(3)
SETTINGS index_granularity = 8192;

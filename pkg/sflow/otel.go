// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/unifabric-io/unifabric/pkg/config"
)

type RecordWriter interface {
	Write(ctx context.Context, records []FlowRecord) error
}

// OtelWriter emits each flow record as one OTLP log record to an
// OpenTelemetry Collector, which owns the storage backend. Batching is done
// by the SDK BatchProcessor. Attribute names are flat and match the fields
// of the Elasticsearch index template the chart installs, so backends stay
// queryable without per-panel string munging: derived fields such as
// scaled_bytes, sampler_ip and src_workload are computed here once.
type OtelWriter struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

// NewOtelWriter builds the exporter and provider. The connection is verified
// by exporting an empty batch so an unreachable collector is reported at
// start time instead of silently queueing.
func NewOtelWriter(ctx context.Context, cfg config.SFlowOtelConfig, writer config.SFlowWriterConfig) (*OtelWriter, error) {
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("sflow.otel.timeout: %w", err)
	}
	flushInterval, err := time.ParseDuration(writer.FlushInterval)
	if err != nil {
		return nil, fmt.Errorf("sflow.writer.flushInterval: %w", err)
	}
	exporterOpts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
		otlploggrpc.WithTimeout(timeout),
	}
	if cfg.Insecure {
		exporterOpts = append(exporterOpts, otlploggrpc.WithInsecure())
	}
	exporter, err := otlploggrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, err
	}
	if err := exporter.Export(ctx, nil); err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("probe %s: %w", cfg.Endpoint, err)
	}
	res := resource.NewSchemaless(
		semconv.ServiceName("unifabric-sflow"),
	)
	processor := sdklog.NewBatchProcessor(exporter,
		sdklog.WithMaxQueueSize(writer.QueueSize),
		sdklog.WithExportMaxBatchSize(writer.BatchSize),
		sdklog.WithExportInterval(flushInterval),
		sdklog.WithExportTimeout(timeout),
	)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)
	return &OtelWriter{
		provider: provider,
		logger:   provider.Logger("unifabric-sflow"),
	}, nil
}

// Write emits every record. Emit is non blocking, the processor's own queue
// drops when full, so the batch here is always accepted and the returned
// error reflects the flush of earlier batches.
func (w *OtelWriter) Write(ctx context.Context, records []FlowRecord) error {
	if len(records) == 0 {
		return nil
	}
	for i := range records {
		w.logger.Emit(ctx, flowLogRecord(&records[i]))
	}
	return w.provider.ForceFlush(ctx)
}

func (w *OtelWriter) Close() {
	if w == nil || w.provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = w.provider.Shutdown(ctx)
}

// flowLogRecord converts one flow record into a log record whose attribute
// names match the Elasticsearch index template fields. Integers keep their
// type so backends can index them numerically. Derived fields are
// precomputed so dashboards never reconstruct them per query.
func flowLogRecord(r *FlowRecord) otellog.Record {
	var record otellog.Record
	record.SetTimestamp(r.TimeFlowStart)
	record.SetObservedTimestamp(time.Now())
	record.SetSeverity(otellog.SeverityInfo)
	record.SetEventName("sflow.flow")
	record.SetBody(attribute.StringValue("sflow.flow"))
	scale := r.SamplingRate
	if scale == 0 {
		scale = 1
	}
	record.AddAttributes(
		attribute.Int64("sequence_num", int64(r.SequenceNum)),
		attribute.Int64("sampling_rate", int64(r.SamplingRate)),
		attribute.String("sampler_ip", ipString(r.SamplerAddress)),
		attribute.Int64("bytes", int64(r.Bytes)),
		attribute.Int64("packets", int64(r.Packets)),
		attribute.Int64("scaled_bytes", int64(r.Bytes*scale)),
		attribute.Int64("scaled_packets", int64(r.Packets*scale)),
		attribute.Int64("proto", int64(r.Proto)),
		attribute.String("protocol", ProtocolName(r.Proto)),
		attribute.String("src_ip", ipString(r.SrcAddr)),
		attribute.String("dst_ip", ipString(r.DstAddr)),
		attribute.Int64("src_port", int64(r.SrcPort)),
		attribute.Int64("dst_port", int64(r.DstPort)),
		attribute.String("src_pod_namespace", r.SrcAttribution.PodNamespace),
		attribute.String("src_pod_name", r.SrcAttribution.PodName),
		attribute.String("src_node_name", r.SrcAttribution.NodeName),
		attribute.String("src_pod_top_owner_kind", r.SrcAttribution.TopOwnerKind),
		attribute.String("src_pod_top_owner_namespace", r.SrcAttribution.TopOwnerNamespace),
		attribute.String("src_pod_top_owner_name", r.SrcAttribution.TopOwnerName),
		attribute.String("src_workload", WorkloadID(r.SrcAttribution)),
		attribute.String("dst_pod_namespace", r.DstAttribution.PodNamespace),
		attribute.String("dst_pod_name", r.DstAttribution.PodName),
		attribute.String("dst_node_name", r.DstAttribution.NodeName),
		attribute.String("dst_pod_top_owner_kind", r.DstAttribution.TopOwnerKind),
		attribute.String("dst_pod_top_owner_namespace", r.DstAttribution.TopOwnerNamespace),
		attribute.String("dst_pod_top_owner_name", r.DstAttribution.TopOwnerName),
		attribute.String("dst_workload", WorkloadID(r.DstAttribution)),
	)
	return record
}

// WorkloadID renders the namespace/kind/name identity dashboards group by,
// or an empty string when the flow endpoint resolved to no workload.
func WorkloadID(a EndpointAttribution) string {
	if a.TopOwnerNamespace == "" || a.TopOwnerKind == "" || a.TopOwnerName == "" {
		return ""
	}
	return a.TopOwnerNamespace + "/" + a.TopOwnerKind + "/" + a.TopOwnerName
}

// ProtocolName maps an IP protocol number to the display name used by the
// dashboards. Unknown protocols keep their number.
func ProtocolName(proto uint32) string {
	switch proto {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 58:
		return "IPv6-ICMP"
	case 132:
		return "SCTP"
	default:
		return "Protocol " + strconv.FormatUint(uint64(proto), 10)
	}
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

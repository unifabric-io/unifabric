// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// otelConfig is how to reach an OpenTelemetry Collector over OTLP gRPC.
type otelConfig struct {
	endpoint string
	insecure bool
	timeout  time.Duration
	node     string
}

// otelSink emits each send row as one OTLP log record. Batching is done by
// the SDK BatchProcessor so a single insert call is cheap, the exporter
// retries transient failures itself. Attribute names are flat and match the
// column names the Collector writes so backends stay queryable the same way.
type otelSink struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

func (s *otelSink) name() string {
	return "otlp"
}

// otelFactory returns the factory for the OTLP sink.
func otelFactory(cfg otelConfig, opts sendOptions) sinkFactory {
	return sinkFactory{
		kind: "otlp",
		open: func(ctx context.Context) (sendSink, error) {
			return newOtelSink(ctx, cfg, opts)
		},
		describe: fmt.Sprintf("endpoint=%q insecure=%t timeout=%s", cfg.endpoint, cfg.insecure, cfg.timeout),
	}
}

// newOtelSink builds the exporter and provider. The connection is verified
// by exporting an empty batch so an unreachable collector is reported at
// connect time instead of silently queueing.
func newOtelSink(ctx context.Context, cfg otelConfig, opts sendOptions) (*otelSink, error) {
	exporterOpts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.endpoint),
		otlploggrpc.WithTimeout(cfg.timeout),
	}
	if cfg.insecure {
		exporterOpts = append(exporterOpts, otlploggrpc.WithInsecure())
	}
	exporter, err := otlploggrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, err
	}
	err = exporter.Export(ctx, nil)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("probe %s: %w", cfg.endpoint, err)
	}
	res := resource.NewSchemaless(
		semconv.ServiceName("unifabric-agent"),
		semconv.K8SNodeName(cfg.node),
	)
	processor := sdklog.NewBatchProcessor(exporter,
		sdklog.WithMaxQueueSize(opts.queue),
		sdklog.WithExportMaxBatchSize(opts.batch),
		sdklog.WithExportInterval(opts.flushIn),
		sdklog.WithExportTimeout(cfg.timeout),
	)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)
	return &otelSink{
		provider: provider,
		logger:   provider.Logger("unifabric-agent"),
	}, nil
}

// insert emits every row. Emit is non blocking, the processor's own queue
// drops when full, so the batch here is always accepted and the returned
// error reflects the flush of earlier batches.
func (s *otelSink) insert(ctx context.Context, rows []sendRow) error {
	for i := range rows {
		s.logger.Emit(ctx, sendRecord(&rows[i]))
	}
	return s.provider.ForceFlush(ctx)
}

// sendRecord converts one row into a log record with the same field names
// as the ClickHouse table. Integers keep their type so backends can index
// them numerically.
func sendRecord(row *sendRow) otellog.Record {
	var record otellog.Record
	record.SetTimestamp(row.ts)
	record.SetObservedTimestamp(time.Now())
	record.SetSeverity(otellog.SeverityInfo)
	record.SetEventName("rdma.send")
	record.SetBody(attribute.StringValue("rdma.send"))
	record.AddAttributes(
		attribute.String("node", row.node),
		attribute.Int64("tgid", int64(row.tgid)),
		attribute.Int64("qpn", int64(row.qpn)),
		attribute.Int64("bytes", int64(row.bytes)),
		attribute.Int64("wrs", int64(row.wrs)),
		attribute.Int64("count", int64(row.count)),
		attribute.String("kind", callbackKindName(row.kind)),
		attribute.String("src_device", row.srcDevice),
		attribute.String("dst_device", row.dstDevice),
		attribute.String("src_pod_namespace", row.src.Namespace),
		attribute.String("src_pod_name", row.src.Name),
		attribute.String("src_pod_top_owner_kind", row.src.Owner.Kind),
		attribute.String("src_pod_top_owner_namespace", row.src.Owner.Namespace),
		attribute.String("src_pod_top_owner_name", row.src.Owner.Name),
		attribute.String("dst_pod_namespace", row.dst.Namespace),
		attribute.String("dst_pod_name", row.dst.Name),
		attribute.String("dst_pod_top_owner_kind", row.dst.Owner.Kind),
		attribute.String("dst_pod_top_owner_namespace", row.dst.Owner.Namespace),
		attribute.String("dst_pod_top_owner_name", row.dst.Owner.Name),
	)
	return record
}

func (s *otelSink) close() {
	if s == nil || s.provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.provider.Shutdown(ctx)
}

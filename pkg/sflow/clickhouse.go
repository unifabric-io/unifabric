// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/unifabric-io/unifabric/pkg/config"
)

type RecordWriter interface {
	Write(ctx context.Context, records []FlowRecord) error
}

type ClickHouseWriter struct {
	Conn  chdriver.Conn
	Table string
}

func NewClickHouseConn(ctx context.Context, cfg config.SFlowClickHouseConfig) (chdriver.Conn, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    5,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (w ClickHouseWriter) Write(ctx context.Context, records []FlowRecord) error {
	if len(records) == 0 {
		return nil
	}
	if w.Conn == nil {
		return fmt.Errorf("clickhouse connection is nil")
	}
	table := strings.TrimSpace(w.Table)
	if table == "" {
		table = "flows_raw"
	}
	batch, err := w.Conn.PrepareBatch(ctx, insertSQL(table))
	if err != nil {
		return err
	}
	for _, r := range records {
		row := RowFromRecord(r)
		if err := batch.Append(row.values()...); err != nil {
			_ = batch.Close()
			return err
		}
	}
	return batch.Send()
}

type FlowRow struct {
	Type                    int32
	TimeReceived            time.Time
	SequenceNum             uint32
	SamplingRate            uint64
	SamplerAddress          string
	TimeFlowStart           time.Time
	TimeFlowEnd             time.Time
	Bytes                   uint64
	Packets                 uint64
	SrcAddr                 string
	DstAddr                 string
	SrcAS                   uint32
	DstAS                   uint32
	Etype                   uint32
	Proto                   uint32
	SrcPort                 uint32
	DstPort                 uint32
	SrcK8sPodName           string
	SrcK8sPodNamespace      string
	SrcK8sNodeName          string
	DstK8sPodName           string
	DstK8sPodNamespace      string
	DstK8sNodeName          string
	SrcK8sTopOwnerKind      string
	SrcK8sTopOwnerName      string
	SrcK8sTopOwnerNamespace string
	DstK8sTopOwnerKind      string
	DstK8sTopOwnerName      string
	DstK8sTopOwnerNamespace string
}

func RowFromRecord(r FlowRecord) FlowRow {
	return FlowRow{
		Type:                    r.Type,
		TimeReceived:            r.TimeReceived,
		SequenceNum:             r.SequenceNum,
		SamplingRate:            r.SamplingRate,
		SamplerAddress:          Fixed16String(r.SamplerAddress),
		TimeFlowStart:           r.TimeFlowStart,
		TimeFlowEnd:             r.TimeFlowEnd,
		Bytes:                   r.Bytes,
		Packets:                 r.Packets,
		SrcAddr:                 Fixed16String(r.SrcAddr),
		DstAddr:                 Fixed16String(r.DstAddr),
		SrcAS:                   r.SrcAS,
		DstAS:                   r.DstAS,
		Etype:                   r.Etype,
		Proto:                   r.Proto,
		SrcPort:                 r.SrcPort,
		DstPort:                 r.DstPort,
		SrcK8sPodName:           r.SrcAttribution.PodName,
		SrcK8sPodNamespace:      r.SrcAttribution.PodNamespace,
		SrcK8sNodeName:          r.SrcAttribution.NodeName,
		DstK8sPodName:           r.DstAttribution.PodName,
		DstK8sPodNamespace:      r.DstAttribution.PodNamespace,
		DstK8sNodeName:          r.DstAttribution.NodeName,
		SrcK8sTopOwnerKind:      r.SrcAttribution.TopOwnerKind,
		SrcK8sTopOwnerName:      r.SrcAttribution.TopOwnerName,
		SrcK8sTopOwnerNamespace: r.SrcAttribution.TopOwnerNamespace,
		DstK8sTopOwnerKind:      r.DstAttribution.TopOwnerKind,
		DstK8sTopOwnerName:      r.DstAttribution.TopOwnerName,
		DstK8sTopOwnerNamespace: r.DstAttribution.TopOwnerNamespace,
	}
}

func (r FlowRow) values() []any {
	return []any{
		r.Type, r.TimeReceived, r.SequenceNum, r.SamplingRate, r.SamplerAddress,
		r.TimeFlowStart, r.TimeFlowEnd, r.Bytes, r.Packets, r.SrcAddr, r.DstAddr,
		r.SrcAS, r.DstAS, r.Etype, r.Proto, r.SrcPort, r.DstPort,
		r.SrcK8sPodName, r.SrcK8sPodNamespace, r.SrcK8sNodeName,
		r.DstK8sPodName, r.DstK8sPodNamespace, r.DstK8sNodeName,
		r.SrcK8sTopOwnerKind, r.SrcK8sTopOwnerName, r.SrcK8sTopOwnerNamespace,
		r.DstK8sTopOwnerKind, r.DstK8sTopOwnerName, r.DstK8sTopOwnerNamespace,
	}
}

func insertSQL(table string) string {
	return fmt.Sprintf(`INSERT INTO %s (
		type, time_received, sequence_num, sampling_rate, sampler_address,
		time_flow_start, time_flow_end, bytes, packets, src_addr, dst_addr,
		src_as, dst_as, etype, proto, src_port, dst_port,
		src_k8s_pod_name, src_k8s_pod_namespace, src_k8s_node_name,
		dst_k8s_pod_name, dst_k8s_pod_namespace, dst_k8s_node_name,
		src_k8s_top_owner_kind, src_k8s_top_owner_name, src_k8s_top_owner_namespace,
		dst_k8s_top_owner_kind, dst_k8s_top_owner_name, dst_k8s_top_owner_namespace
	)`, table)
}

// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
)

type Collector struct {
	ListenAddr    string
	Decoder       Decoder
	Lookup        PodLookup
	Writer        RecordWriter
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int
	Metrics       *CollectorMetrics
	Log           *slog.Logger
}

func NewCollector(cfg *config.SFlowConfig, lookup PodLookup, writer RecordWriter, log *slog.Logger) (*Collector, error) {
	flushInterval, err := time.ParseDuration(cfg.Writer.FlushInterval)
	if err != nil {
		return nil, err
	}
	return &Collector{
		ListenAddr:    cfg.Listen.BindAddress,
		Decoder:       Decoder{},
		Lookup:        lookup,
		Writer:        writer,
		BatchSize:     cfg.Writer.BatchSize,
		FlushInterval: flushInterval,
		QueueSize:     cfg.Writer.QueueSize,
		Metrics:       &CollectorMetrics{},
		Log:           log,
	}, nil
}

func (c *Collector) Run(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", c.ListenAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.ServeUDP(ctx, conn)
}

func (c *Collector) ServeUDP(ctx context.Context, conn *net.UDPConn) error {
	if c.Writer == nil {
		return errors.New("record writer is nil")
	}
	queueSize := c.QueueSize
	if queueSize <= 0 {
		queueSize = 1
	}
	rowsCh := make(chan FlowRecord, queueSize)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		c.writeLoop(ctx, rowsCh)
	}()

	buf := make([]byte, 65535)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if isTimeout(err) {
				select {
				case <-ctx.Done():
					close(rowsCh)
					<-writerDone
					return nil
				default:
					continue
				}
			}
			select {
			case <-ctx.Done():
				close(rowsCh)
				<-writerDone
				return nil
			default:
				if c.Log != nil {
					c.Log.Warn("read sflow datagram failed", "error", err)
				}
				continue
			}
		}

		if c.Metrics != nil {
			c.Metrics.DatagramsAccepted.Add(1)
		}
		records, err := c.Decoder.DecodeDatagram(buf[:n], remote)
		if err != nil {
			if c.Metrics != nil {
				c.Metrics.DecodeErrors.Add(1)
			}
			if c.Log != nil {
				c.Log.Warn("decode sflow datagram failed", "remote", remote.String(), "error", err)
			}
			continue
		}
		if c.Metrics != nil {
			c.Metrics.RecordsDecoded.Add(uint64(len(records)))
		}
		for _, record := range records {
			record = EnrichRecord(record, c.Lookup)
			select {
			case rowsCh <- record:
				if c.Metrics != nil {
					c.Metrics.QueueDepth.Add(1)
				}
			case <-ctx.Done():
				close(rowsCh)
				<-writerDone
				return nil
			default:
				if c.Metrics != nil {
					c.Metrics.RecordsDropped.Add(1)
				}
				if c.Log != nil {
					c.Log.Warn("drop sflow record because writer queue is full")
				}
			}
		}
	}
}

func (c *Collector) writeLoop(ctx context.Context, rowsCh <-chan FlowRecord) {
	ticker := time.NewTicker(c.FlushInterval)
	defer ticker.Stop()
	batchSize := c.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := make([]FlowRecord, 0, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := c.Writer.Write(ctx, batch); err != nil {
			if c.Metrics != nil {
				c.Metrics.WriteErrors.Add(1)
			}
			if c.Log != nil {
				c.Log.Error("write sflow records failed", "records", len(batch), "error", err)
			}
		} else if c.Metrics != nil {
			c.Metrics.addWritten(len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case row, ok := <-rowsCh:
			if !ok {
				flush()
				return
			}
			if c.Metrics != nil {
				c.Metrics.QueueDepth.Add(-1)
			}
			batch = append(batch, row)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

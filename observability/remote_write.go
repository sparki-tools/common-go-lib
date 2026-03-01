package observability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"go.uber.org/zap"
)

// RemoteWriteConfig configures the Prometheus remote write client
type RemoteWriteConfig struct {
	// Endpoint is the remote write URL (e.g., Grafana Cloud)
	Endpoint string
	// Username for basic auth (Grafana Cloud instance ID)
	Username string
	// Password for basic auth (Grafana Cloud API key)
	Password string
	// PushInterval controls how often metrics are pushed
	PushInterval time.Duration
	// Timeout for HTTP requests
	Timeout time.Duration
	// Labels to add to all metrics
	ExternalLabels map[string]string
}

// RemoteWriteClient pushes metrics to a Prometheus remote write endpoint
type RemoteWriteClient struct {
	config  *RemoteWriteConfig
	client  *http.Client
	logger  *zap.Logger
	stopCh  chan struct{}
	running bool
}

// NewRemoteWriteClient creates a new remote write client
func NewRemoteWriteClient(config *RemoteWriteConfig, logger *zap.Logger) *RemoteWriteClient {
	if config.PushInterval == 0 {
		config.PushInterval = 15 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	// Log configuration at startup (mask password for security)
	maskedPassword := ""
	if config.Password != "" {
		maskedPassword = config.Password[:min(4, len(config.Password))] + "****"
	}
	logger.Info("remote write client configured",
		zap.String("endpoint", config.Endpoint),
		zap.String("username", config.Username),
		zap.String("password_preview", maskedPassword),
		zap.Duration("push_interval", config.PushInterval),
	)

	return &RemoteWriteClient{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start begins the periodic push to remote write endpoint
func (c *RemoteWriteClient) Start(ctx context.Context) {
	if c.running {
		return
	}
	c.running = true

	go func() {
		ticker := time.NewTicker(c.config.PushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := c.Push(ctx); err != nil {
					c.logger.Error("failed to push metrics", zap.Error(err))
				}
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop halts the periodic push
func (c *RemoteWriteClient) Stop() {
	if c.running {
		close(c.stopCh)
		c.running = false
	}
}

// Push collects and sends metrics to the remote write endpoint
func (c *RemoteWriteClient) Push(ctx context.Context) error {
	// Gather all metrics from the default registry
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return fmt.Errorf("gathering metrics: %w", err)
	}

	// Convert to remote write format
	timeSeries := c.metricsToTimeSeries(mfs)
	if len(timeSeries) == 0 {
		return nil
	}

	// Create write request
	req := &prompb.WriteRequest{
		Timeseries: timeSeries,
	}

	// Marshal and compress
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	compressed := snappy.Encode(nil, data)

	// Send HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if c.config.Username != "" && c.config.Password != "" {
		httpReq.SetBasicAuth(c.config.Username, c.config.Password)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		c.logger.Error("remote write failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)),
			zap.String("endpoint", c.config.Endpoint),
			zap.String("username", c.config.Username),
		)
		return fmt.Errorf("remote write failed with status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("pushed metrics to remote write",
		zap.Int("series_count", len(timeSeries)),
		zap.String("endpoint", c.config.Endpoint),
	)

	return nil
}

// metricsToTimeSeries converts Prometheus metrics to remote write time series
func (c *RemoteWriteClient) metricsToTimeSeries(mfs []*dto.MetricFamily) []prompb.TimeSeries {
	var result []prompb.TimeSeries
	now := time.Now().UnixMilli()

	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			labels := []prompb.Label{{Name: "__name__", Value: mf.GetName()}}

			// Add metric labels
			for _, lp := range m.GetLabel() {
				labels = append(labels, prompb.Label{
					Name:  lp.GetName(),
					Value: lp.GetValue(),
				})
			}

			// Add external labels
			for k, v := range c.config.ExternalLabels {
				labels = append(labels, prompb.Label{Name: k, Value: v})
			}

			// Extract value based on metric type
			var value float64
			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				value = m.GetCounter().GetValue()
			case dto.MetricType_GAUGE:
				value = m.GetGauge().GetValue()
			case dto.MetricType_UNTYPED:
				value = m.GetUntyped().GetValue()
			case dto.MetricType_HISTOGRAM:
				// For histograms, emit _sum, _count, and bucket metrics
				h := m.GetHistogram()
				result = append(result, c.histogramToTimeSeries(mf.GetName(), labels, h, now)...)
				continue
			case dto.MetricType_SUMMARY:
				// For summaries, emit _sum, _count, and quantile metrics
				s := m.GetSummary()
				result = append(result, c.summaryToTimeSeries(mf.GetName(), labels, s, now)...)
				continue
			default:
				continue
			}

			result = append(result, prompb.TimeSeries{
				Labels:  labels,
				Samples: []prompb.Sample{{Value: value, Timestamp: now}},
			})
		}
	}

	return result
}

func (c *RemoteWriteClient) histogramToTimeSeries(name string, baseLabels []prompb.Label, h *dto.Histogram, timestamp int64) []prompb.TimeSeries {
	var result []prompb.TimeSeries

	// _sum
	sumLabels := make([]prompb.Label, len(baseLabels))
	copy(sumLabels, baseLabels)
	sumLabels[0] = prompb.Label{Name: "__name__", Value: name + "_sum"}
	result = append(result, prompb.TimeSeries{
		Labels:  sumLabels,
		Samples: []prompb.Sample{{Value: h.GetSampleSum(), Timestamp: timestamp}},
	})

	// _count
	countLabels := make([]prompb.Label, len(baseLabels))
	copy(countLabels, baseLabels)
	countLabels[0] = prompb.Label{Name: "__name__", Value: name + "_count"}
	result = append(result, prompb.TimeSeries{
		Labels:  countLabels,
		Samples: []prompb.Sample{{Value: float64(h.GetSampleCount()), Timestamp: timestamp}},
	})

	// _bucket
	for _, b := range h.GetBucket() {
		bucketLabels := make([]prompb.Label, len(baseLabels))
		copy(bucketLabels, baseLabels)
		bucketLabels[0] = prompb.Label{Name: "__name__", Value: name + "_bucket"}
		bucketLabels = append(bucketLabels, prompb.Label{
			Name:  "le",
			Value: fmt.Sprintf("%g", b.GetUpperBound()),
		})
		result = append(result, prompb.TimeSeries{
			Labels:  bucketLabels,
			Samples: []prompb.Sample{{Value: float64(b.GetCumulativeCount()), Timestamp: timestamp}},
		})
	}

	return result
}

func (c *RemoteWriteClient) summaryToTimeSeries(name string, baseLabels []prompb.Label, s *dto.Summary, timestamp int64) []prompb.TimeSeries {
	var result []prompb.TimeSeries

	// _sum
	sumLabels := make([]prompb.Label, len(baseLabels))
	copy(sumLabels, baseLabels)
	sumLabels[0] = prompb.Label{Name: "__name__", Value: name + "_sum"}
	result = append(result, prompb.TimeSeries{
		Labels:  sumLabels,
		Samples: []prompb.Sample{{Value: s.GetSampleSum(), Timestamp: timestamp}},
	})

	// _count
	countLabels := make([]prompb.Label, len(baseLabels))
	copy(countLabels, baseLabels)
	countLabels[0] = prompb.Label{Name: "__name__", Value: name + "_count"}
	result = append(result, prompb.TimeSeries{
		Labels:  countLabels,
		Samples: []prompb.Sample{{Value: float64(s.GetSampleCount()), Timestamp: timestamp}},
	})

	// quantiles
	for _, q := range s.GetQuantile() {
		qLabels := make([]prompb.Label, len(baseLabels))
		copy(qLabels, baseLabels)
		qLabels = append(qLabels, prompb.Label{
			Name:  "quantile",
			Value: fmt.Sprintf("%g", q.GetQuantile()),
		})
		result = append(result, prompb.TimeSeries{
			Labels:  qLabels,
			Samples: []prompb.Sample{{Value: q.GetValue(), Timestamp: timestamp}},
		})
	}

	return result
}

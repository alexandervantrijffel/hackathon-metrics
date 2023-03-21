package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/instrument"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	log "github.com/sirupsen/logrus"
)

const (
	service string = "metrics_example"
	team    string = "team_awesome"

	grpcCollectorAddressEnvKey string = "OTEL_GRPC_COLLECTOR_ADDRESS"
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.Info("Starting Golang Metrics Example Service")

	// Set up termination logic
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	defer func() {
		log.Debug("Waiting for goroutines to finish")
		cancel()
		wg.Wait()
		log.Debug("Goroutines finished")
	}()

	// Set up attributes (tags) and resource for metrics and traces
	attrs := []attribute.KeyValue{
		attribute.Key("service").String(service),
		attribute.Key("team").String(team),
	}
	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			attrs...,
		),
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create resource")
	}

	// Set up the Prometheus metrics exporter
	metricsExporter, err := prometheus.New()
	if err != nil {
		log.Fatal(err)
	}

	// Set up the OpenTelemetry metrics stack
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricsExporter),
		sdkmetric.WithResource(res),
	)
	meter := meterProvider.Meter(service)

	// Start the Prometheus HTTP server
	go serveMetrics()

	var tracer trace.Tracer

	// Set up the OpenTelemetry gRPC trace exporter
	conn, err := grpc.DialContext(ctx, os.Getenv(grpcCollectorAddressEnvKey),
		// Note the use of insecure transport here. TLS is recommended in production.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.WithError(err).Error("Failed to create gRPC connection to collector")
	} else {
		log.Info("Set up tracing - 1")
		// Set up the OpenTelemetry tracing stack
		// Set up a trace exporter
		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			log.WithError(err).Fatal("Failed to create trace exporter: %w", err)
		}

		log.Info("Set up tracing - 2")

		// Register the trace exporter with a TracerProvider, using a batch
		// span processor to aggregate spans before export.
		bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithResource(res),
			sdktrace.WithSpanProcessor(bsp),
		)
		otel.SetTracerProvider(tracerProvider)

		// set global propagator to tracecontext (the default is no-op).
		// otel.SetTextMapPropagator(propagation.TraceContext{})
		log.Info("Set up tracing - 3")

		tracer = otel.Tracer(service)
	}

	generateTrigonoMetrics(meter, &wg, ctx)
	generateStandardHTTPMetrics(tracer, &wg, ctx)
	generateGorillaHTTPMetrics(tracer, &wg, ctx)

	waitForInterrupt()
}

func serveMetrics() {
	log.Infof("Serving metrics at localhost:2223/metrics")
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":2223", nil)
	if err != nil {
		log.WithError(err).Fatal("Error serving metrics over HTTP")
	}
}

func generateTrigonoMetrics(meter metric.Meter, wg *sync.WaitGroup, ctx context.Context) {
	interval := time.Second
	log.Infof("Starting trigonometric metrics generation every %s", interval)

	ticker := time.NewTicker(interval)

	wg.Add(1)
	go func() {
		var degrees int
		var ticks int
		var radians float64
		multiplier := 100.

		trigPeriod := int(time.Minute / interval) // Absolute trigonometric period; change every minute

		// Create metrics
		gauge, err := meter.Float64ObservableGauge("trigonometric_gauge", instrument.WithDescription("trigonometric gauge"))
		if err != nil {
			log.Fatal(err)
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
			log.Trace("Observing trigonometric gauge")
			o.ObserveFloat64(gauge, math.Abs(math.Sin(radians))*multiplier, attribute.Key("function").String("sine"))
			o.ObserveFloat64(gauge, math.Abs(math.Cos(radians))*multiplier, attribute.Key("function").String("cosine"))
			return nil
		}, gauge)
		if err != nil {
			log.Fatal(err)
		}

		counter, err := meter.Float64Counter("trigonometric_counter", instrument.WithDescription("trigonometric counter"))
		if err != nil {
			log.Fatal(err)
		}

		for {
			select {
			case <-ticker.C:
				if ticks == trigPeriod {
					log.Trace("Generating new trigonometric values")
					if degrees < 360 {
						degrees += 10
					} else {
						degrees = 0
					}
					ticks = 0
				}
				ticks++

				radians = (float64(degrees) * math.Pi) / 180
				counter.Add(ctx, 1)

			case <-ctx.Done():
				log.Info("Stopping trigonometric metrics generation")
				ticker.Stop()
				wg.Done()
				return
			}
		}
	}()
}

func getRandomStatus() int {
	r := rand.Intn(100)
	if r >= 80 { // Return an error
		r = rand.Intn(2)
		if r > 0 {
			return http.StatusInternalServerError
		}
		return http.StatusBadRequest
	}
	return http.StatusOK
}

func getHTTPHandlerFunc(tracer trace.Tracer, traceName string, minDelay int, maxDelay int) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tracer != nil {
			ctx := r.Context()
			_, span := tracer.Start(ctx, traceName)
			defer span.End()
		}

		sleep := rand.Intn(maxDelay-minDelay) + minDelay
		status := getRandomStatus()
		log.Tracef("HTTP request received, sleeping %dms and returning %d", sleep, status)
		time.Sleep(time.Millisecond * time.Duration(sleep))
		w.WriteHeader(getRandomStatus())
	})
}

func doHTTPRequest(tracer trace.Tracer, client *http.Client, uri *url.URL, pattern string, ctx context.Context) error {
	log.Tracef("Doing HTTP request to %s", uri)

	if tracer != nil {
		var span trace.Span
		ctx, span = tracer.Start(ctx, pattern, trace.WithAttributes(semconv.PeerService(service)))
		defer span.End()

		ctx = httptrace.WithClientTrace(ctx, otelhttptrace.NewClientTrace(ctx))
	}

	// Set up request
	req, err := http.NewRequestWithContext(ctx, "GET", uri.String(), nil)
	if err != nil {
		return err
	}

	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func generateStandardHTTPMetrics(tracer trace.Tracer, wg *sync.WaitGroup, ctx context.Context) {
	interval := time.Millisecond * 500
	log.Infof("Starting standard HTTP metrics generation every %s", interval)

	// Set up server
	min := 10
	max := 100

	// Set up HTTP server metrics
	server := httptest.NewServer(getHTTPHandlerFunc(tracer, "serve_root", min, max))

	wg.Add(1)
	go func() {
		// Set up client
		client := http.DefaultClient

		// Set up loop
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				uri, _ := url.ParseRequestURI(server.URL)
				uri = uri.JoinPath("/")
				if err := doHTTPRequest(tracer, client, uri, "get_root", ctx); err != nil {
					log.WithError(err).Errorf("Cannot do request to %s", server.URL)
				}
			case <-ctx.Done():
				log.Info("Stopping standard HTTP metrics generation")
				ticker.Stop()
				server.Close()
				wg.Done()
				return
			}
		}
	}()
}

func generateGorillaHTTPMetrics(tracer trace.Tracer, wg *sync.WaitGroup, ctx context.Context) {
	interval := time.Millisecond * 500
	log.Infof("Starting Gorilla HTTP metrics generation every %s", interval)

	// Set up server
	r := mux.NewRouter()

	// Add paths
	r.Methods("GET").Path("/").HandlerFunc(getHTTPHandlerFunc(tracer, "serve_root", 10, 100))
	r.Methods("GET").Path("/test1").HandlerFunc(getHTTPHandlerFunc(tracer, "serve_test1", 50, 150))
	r.Methods("GET").Path("/test1/{resource1}").HandlerFunc(getHTTPHandlerFunc(tracer, "serve_test1_resource1", 20, 50))

	// Get random port for server and serve
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		log.Tracef("Gorilla Mux server listening on %d", port)
		if err := http.Serve(listener, r); err != nil {
			log.Panicf("Error while serving: %s", err)
		}
	}()

	wg.Add(1)
	go func() {
		// Set up client
		client := http.DefaultClient

		// Set up loop
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				host := fmt.Sprintf("http://127.0.0.1:%d", port)
				patterns := map[string]string{
					"get_root":            "/",
					"get_test1":           "/test1",
					"get_test1_resource1": "/test1/12345",
				}
				for pattern, path := range patterns {
					uri, _ := url.ParseRequestURI(host)
					uri = uri.JoinPath(path)
					if err != nil {
						log.WithError(err).Errorf("Cannot compose URL %s%s", host, path)
					}
					if err := doHTTPRequest(tracer, client, uri, pattern, ctx); err != nil {
						log.WithError(err).Errorf("Cannot do request to %s", uri)
					}
				}
			case <-ctx.Done():
				log.Info("Stopping Gorilla HTTP metrics generation")
				ticker.Stop()
				wg.Done()
				return
			}
		}
	}()
}

func waitForInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer signal.Stop(c)

	<-c
	log.Debug("Interrupt signal received")
}

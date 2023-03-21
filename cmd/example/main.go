package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/sdk/metric"

	log "github.com/sirupsen/logrus"
)

const (
	service string = "metrics_example"
	team    string = "team_awesome"
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.Info("Starting Golang Metrics Example Service")

	// Set up the Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatal(err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	meter := provider.Meter(service)

	// Start the Prometheus HTTP server
	go serveMetrics()

	// Set up attributes (tags)
	attrs := []attribute.KeyValue{
		attribute.Key("service").String(service),
		attribute.Key("team").String(team),
	}

	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		log.Debug("Waiting for goroutines to finish")
		cancel()
		wg.Wait()
		log.Debug("Goroutines finished")
	}()

	generateTrigonoMetrics(meter, attrs, &wg, ctx)
	generateStandardHTTPMetrics(meter, attrs, &wg, ctx)
	generateGorillaHTTPMetrics(meter, attrs, &wg, ctx)

	waitForInterrupt()
}

func serveMetrics() {
	log.Printf("serving metrics at localhost:2223/metrics")
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":2223", nil)
	if err != nil {
		log.WithError(err).Fatal("Error serving metrics over HTTP")
	}
}

func generateTrigonoMetrics(meter api.Meter, attrs []attribute.KeyValue, wg *sync.WaitGroup, ctx context.Context) {
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
		if err := observableGauge(meter, "sine", "sine gauge", attrs, func() float64 {
			return math.Abs(math.Sin(radians)) * multiplier
		}); err != nil {
			log.Fatal(err)
		}

		if err := observableGauge(meter, "cosine", "cosine gauge", attrs, func() float64 {
			return math.Abs(math.Cos(radians)) * multiplier
		}); err != nil {
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

				counter.Add(ctx, 1, attrs...)
			case <-ctx.Done():
				log.Info("Stopping trigonometric metrics generation")
				ticker.Stop()
				wg.Done()
				return
			}
		}
	}()
}

func observableGauge(meter api.Meter, name string, description string, attrs []attribute.KeyValue, valueFunc func() float64) error {
	gauge, err := meter.Float64ObservableGauge(name, instrument.WithDescription(description))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
		o.ObserveFloat64(gauge, valueFunc(), attrs...)
		return nil
	}, gauge)
	if err != nil {
		log.Fatal(err)
	}
	return nil
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

func getHTTPHandlerFunc(minDelay int, maxDelay int) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sleep := rand.Intn(maxDelay-minDelay) + minDelay
		status := getRandomStatus()
		log.Tracef("HTTP request received, sleeping %dms and returning %d", sleep, status)
		time.Sleep(time.Millisecond * time.Duration(sleep))
		w.WriteHeader(getRandomStatus())
	})
}

func doHTTPRequest(client *http.Client, uri *url.URL, pattern string) error {
	log.Tracef("Doing HTTP request to %s", uri)

	// Set up request
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return err
	}

	// TODO: measure HTTP request time with metrics SDK
	// pm := httpmetrics.NewPathPatternMatcher()
	// pm.AddPattern(pattern)

	// ctx, trace, err := httpmetrics.WithPatternMatchingClientTrace(req.Context(), cm, "ExampleHTTPClientMetric", pm)
	// if err != nil {
	// 	return err
	// }
	// req = req.WithContext(ctx)

	// Do request
	// res, err := client.Do(req)
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	// End trace
	// err = trace.End(res)
	// if err != nil {
	// 	return err
	// }

	return nil
}

func generateStandardHTTPMetrics(meter api.Meter, attrs []attribute.KeyValue, wg *sync.WaitGroup, ctx context.Context) {
	interval := time.Millisecond * 500
	log.Infof("Starting standard HTTP metrics generation every %s", interval)

	// Set up server
	min := 10
	max := 100

	// TODO: set up HTTP server metrics
	// mw, _ := httpmetrics.NewMiddleware(cm, "ExampleStandardHTTPServerMetric")
	// handler, err := httpmetrics.Handler(
	// 	mw,
	// 	getHTTPHandlerFunc(min, max),
	// 	"/",
	// )
	// if err != nil {
	// 	log.WithError(err).Fatal("Cannot initialize standard HTTP metrics handler")
	// }

	// server := httptest.NewServer(handler)

	server := httptest.NewServer(getHTTPHandlerFunc(min, max))

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
				if err := doHTTPRequest(client, uri, "/"); err != nil {
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

func generateGorillaHTTPMetrics(meter api.Meter, attrs []attribute.KeyValue, wg *sync.WaitGroup, ctx context.Context) {
	interval := time.Millisecond * 500
	log.Infof("Starting Gorilla HTTP metrics generation every %s", interval)

	// Set up server
	// Create our router with the metrics middleware
	// TODO: add HTTP server metrics middleware
	// mw, _ := httpmetrics.NewMiddleware(cm, "ExampleGorillaHTTPServerMetric")
	r := mux.NewRouter()
	// r.Use(httpmetrics.HandlerProvider(mw))

	// Add paths
	r.Methods("GET").Path("/").HandlerFunc(getHTTPHandlerFunc(10, 100))
	r.Methods("GET").Path("/test1").HandlerFunc(getHTTPHandlerFunc(50, 150))
	r.Methods("GET").Path("/test1/{resource1}").HandlerFunc(getHTTPHandlerFunc(20, 50))

	// Add paths to middleware
	// If the paths are not added, they are not monitored
	// mw.ObservePath("/")
	// mw.ObservePath("/test1")
	// mw.ObservePath("/test1/{resource1}")

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
					"/":                  "/",
					"/test1":             "/test1",
					"/test1/{resource1}": "/test1/12345",
				}
				for pattern, path := range patterns {
					uri, _ := url.ParseRequestURI(host)
					uri = uri.JoinPath(path)
					if err != nil {
						log.WithError(err).Errorf("Cannot compose URL %s%s", host, path)
					}
					if err := doHTTPRequest(client, uri, pattern); err != nil {
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

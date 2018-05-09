package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

var (
	bind = flag.String("bind", "127.0.0.1:8080", "bind to this socket")

	self = flag.String("self", "http://localhost:8080", "This should be a valid base URL that points to the current server, for example \"http://example.net:8000\".")

	manualPeers = flag.String("peers", "", "CSV separated list of peers' URLs")
	srvDNSName  = flag.String("peer-srv-endpoint", "", "SRV record prefix for peer discovery (intended for use with kubernetes headless services)")

	bucket = flag.String("bucket", "", "Bucket ot use for S3 client")
)

func parseArgs() {
	flag.Parse()

	if *bucket == "" {
		log.Fatal("-bucket is required")
	}

	if _, err := url.Parse(*self); err != nil {
		log.Fatalf("-self=%q does not contain a valid URL: %s", *self, err)
	}

	if *manualPeers != "" && *srvDNSName != "" {
		log.Fatal("-peers & -peer-srv-endpoint are mututally exclusive options")
	}

	if peers := strings.Split(*manualPeers, ","); len(peers) > 0 {
		for _, p := range peers {
			_, err := url.Parse(p)
			if err != nil {
				log.Fatalf("%q is not a valid URL", p)
			}
		}
	}
}

func main() {
	parseArgs()

	s3c := s3.New(
		session.Must(session.NewSession(&aws.Config{
			Region:           aws.String("us-west-2"),
			S3ForcePathStyle: aws.Bool(true),
			Endpoint:         aws.String("http://localhost:9000"),
		})),
	)

	s3cache := NewS3Cache(s3c, *bucket)

	group := groupcache.NewGroup(
		"bazelcache",
		2<<32,
		s3cache,
	)

	go func() {
		for t := time.Tick(time.Second * 10); ; <-t {
			log.Printf("Stats | %+v", group.Stats)
			log.Printf("CacheStats:MainCache | %+v", group.CacheStats(groupcache.MainCache))
			log.Printf("CacheStats:HotCache | %+v", group.CacheStats(groupcache.HotCache))
		}
	}()

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]

		// log.Printf("%s %s%s", r.Method, *bucket, key)

		switch r.Method {
		case "HEAD", "GET":
			var b groupcache.ByteView
			err := group.Get(nil, key, groupcache.ByteViewSink(&b))
			if err := errors.Cause(err); err != nil {
				if awsErr, ok := err.(awserr.RequestFailure); ok && awsErr.StatusCode() == http.StatusNotFound {
					http.NotFound(rw, r)
					return
				}

				log.Println(errors.Wrap(err, "http get request failed"))
				http.Error(rw, "failed to retrieve key", http.StatusInternalServerError)
			}

			if _, err := io.Copy(rw, b.Reader()); err != nil {
				log.Println(errors.Wrap(err, "error sending http get reply"))
			}
		case "PUT":
			err := s3cache.Put(r.Context(), key, r.Body)

			if err != nil {
				log.Println(errors.Wrap(err, "http put request failed"))
				http.Error(rw, "put failed", http.StatusInternalServerError)
				return
			}
		}
	})

	pool := groupcache.NewHTTPPool(*self)

	switch {
	case *manualPeers != "":
		peers := strings.Split(*manualPeers, ",")
		StaticPeers(pool, append(peers, *self))
	case *srvDNSName != "":
		go func() {
			err := SRVDiscoveredPeers(pool, *self, *srvDNSName)
			log.Fatal("SRV peer resolution has died: ", err)
		}()
	}

	http.ListenAndServe(*bind, nil)
}

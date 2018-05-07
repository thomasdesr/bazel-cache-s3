package main

import (
	"flag"
	"io"
	"log"
	"net/http"
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
	addr   = flag.String("addr", ":8080", "bind to this socket")
	peers  = flag.String("peers", "http://localhost:8080", "List of URLs to resolve peers from")
	bucket = flag.String("bucket", "", "Bucket ot use for S3 client")
)



func parseArgs() {
	flag.Parse()

	if *bucket == "" {
		log.Fatal("-bucket is required")
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
		"bazel-cache",
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
		case "GET":
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

	peers := strings.Split(*peers, ",")
	log.Println("peers", peers)
	pool := groupcache.NewHTTPPool(peers[0])
	pool.Set(peers...)

	http.ListenAndServe(*addr, nil)
}

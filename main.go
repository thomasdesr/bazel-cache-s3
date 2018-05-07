package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

var (
	addr   = flag.String("addr", ":8080", "bind to this socket")
	peers  = flag.String("peers", "http://localhost:8080", "List of URLs to resolve peers from")
	bucket = flag.String("bucket", "", "Bucket ot use for S3 client")
)

func s3Get(client *s3.S3, bucket string, key string) ([]byte, error) {
	r, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to download object")
	}

	var b bytes.Buffer
	if r.ContentLength != nil {
		b.Grow(int(*r.ContentLength))
	}
	if _, err := io.Copy(&b, r.Body); err != nil {
		return nil, errors.Wrap(err, "failed to buffer request")
	}

	return b.Bytes(), nil
}

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
	uploader := s3manager.NewUploaderWithClient(s3c)

	group := groupcache.NewGroup(
		"bazel-cache",
		2<<32,
		groupcache.GetterFunc(func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			b, err := s3Get(s3c, *bucket, key)
			if err != nil {
				return errors.Wrap(err, "failed hydration during s3 get")
			}

			return errors.Wrap(
				dest.SetBytes(b),
				"failed hydration when setting bytes on the groupcache sync",
			)
		}),
	)

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		key := r.URL.Path

		log.Printf("%s %s%s", r.Method, *bucket, key)

		switch r.Method {
		case "GET":
			var b groupcache.ByteView
			err := group.Get(nil, key, groupcache.ByteViewSink(&b))
			if err != nil {
				log.Println(errors.Wrap(err, "http get request failed"))
				http.Error(rw, "failed to retrieve key", http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(rw, b.Reader()); err != nil {
				log.Println(errors.Wrap(err, "error sending http get reply"))
			}
		case "PUT":
			_, err := uploader.UploadWithContext(r.Context(), &s3manager.UploadInput{
				Bucket: bucket,
				Key:    aws.String(key),
				Body:   r.Body,
			})

			if err != nil {
				log.Println(errors.Wrap(err, "http put request failed"))
				http.Error(rw, "put failed", http.StatusInternalServerError)
				return
			}
		}
	})

	peers := strings.Split(*peers, ",")
	pool := groupcache.NewHTTPPool(peers[0])
	pool.Set(peers...)

	http.ListenAndServe(*addr, nil)
}

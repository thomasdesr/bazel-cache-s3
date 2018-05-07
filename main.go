package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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

func s3GetSize(s3c *s3.S3, bucket string, key string) (int64, error) {
	r, err := s3c.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, err
	}

	if r.ContentLength != nil {
		return *r.ContentLength, nil
	}
	return 0, nil
}

func s3Get(dl *s3manager.Downloader, bucket string, key string) ([]byte, error) {
	log.Println("S3 get triggered")

	size, err := s3GetSize(dl.S3.(*s3.S3), bucket, key)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to head object")
	}

	buf := aws.NewWriteAtBuffer(make([]byte, size))
	dl.Download(buf, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to download object")
	}

	return buf.Bytes(), nil
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
	downloader := s3manager.NewDownloaderWithClient(s3c)
	uploader := s3manager.NewUploaderWithClient(s3c)

	group := groupcache.NewGroup(
		"bazel-cache",
		2<<32,
		groupcache.GetterFunc(func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			b, err := s3Get(downloader, *bucket, key)
			if err != nil {
				return errors.Wrap(err, "failed hydration during s3 get")
			}

			return errors.Wrap(
				dest.SetBytes(b),
				"failed hydration when setting bytes on the groupcache sync",
			)
		}),
	)

	go func() {
		for t := time.Tick(time.Second * 10); ; <-t {
			log.Printf("Stats | %+v", group.Stats)
			log.Printf("CacheStats:MainCache | %+v", group.CacheStats(groupcache.MainCache))
			log.Printf("CacheStats:HotCache | %+v", group.CacheStats(groupcache.HotCache))
		}
	}()

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
